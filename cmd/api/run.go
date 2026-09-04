package main

import (
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	entitlements "github.com/my/app/internal/application/entitlements"
	"github.com/my/app/internal/application/workflows/reply"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/logger"
	"github.com/my/app/internal/transport/http/handlers"
	appmiddleware "github.com/my/app/internal/transport/http/middleware"
	"github.com/my/app/internal/transport/http/routes"
)

func run() {
	cfg, err := config.Load()
	if err != nil {
		panic("load config: " + err.Error())
	}

	if err := logger.Init(logger.Config{
		Level:      cfg.Logger.Level,
		Format:     cfg.Logger.Format,
		OutputPath: cfg.Logger.OutputPath,
	}); err != nil {
		panic("init logger: " + err.Error())
	}
	defer func() { _ = logger.Get().Sync() }()

	log := logger.Get()

	platform := buildPlatform(cfg, log)
	defer platform.Close()

	knowledge := buildKnowledge(platform.router)
	workflow := reply.NewWorkflow(knowledge, buildWorkflowOptions(cfg, platform.router, log, platform.resolver)...)

	e := echo.New()

	endpoints := buildAPIHandlers(cfg, platform.central, platform.router, platform.resolver, log)
	keyMaterial := serviceJWTKeyMaterial(cfg, log)
	if cfg.App.IsProduction() && keyMaterial == "" {
		log.Fatal("service JWT key material is required in production; set the Laravel public key path or PEM")
	}
	var features appmiddleware.FeatureReader
	if platform.central != nil {
		features = entitlements.New(entitlements.SQLDB{DB: platform.central}, time.Duration(cfg.Entitlements.CacheTTLSeconds)*time.Second)
	}

	routes.Register(e, handlers.NewReplyHandler(workflow), routes.Options{
		Log:            log,
		SwaggerEnabled: cfg.Swagger.EnabledFor(cfg.App),
		JWTSecret:      keyMaterial,
		RequireAuth:    cfg.App.IsProduction() || !cfg.App.DevAuthBypass,
		Reports:        endpoints.reports,
		Inbound:        endpoints.inbound,
		Feedback:       endpoints.feedback,
		Review:         endpoints.review,
		Chat:           endpoints.chat,
		ChatStream:     endpoints.chatStream,
		ChatConfirm:    endpoints.chatConfirm,
		Search:         endpoints.search,
		Intake:         endpoints.intake,
		Features:       features,
		Budget: appmiddleware.BudgetPolicy{
			Fallback: time.Duration(cfg.Server.RequestBudgetMs) * time.Millisecond,
			Headroom: time.Duration(cfg.Server.DeadlineHeadroomMs) * time.Millisecond,
			Min:      time.Duration(cfg.Server.DeadlineMinMs) * time.Millisecond,
			Max:      time.Duration(cfg.Server.DeadlineMaxMs) * time.Millisecond,
		},
	})

	log.Info("starting server", zap.String("port", cfg.Server.Port))
	if err := e.Start(":" + cfg.Server.Port); err != nil {
		log.Fatal("start api", zap.Error(err))
	}
}
