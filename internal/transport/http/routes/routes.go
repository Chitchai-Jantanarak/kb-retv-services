package routes

import (
	"net/http"

	"github.com/labstack/echo/v5"
	echoSwagger "github.com/swaggo/echo-swagger/v2"
	"go.uber.org/zap"

	_ "github.com/my/app/docs/swagger" // generated swagger docs registration
	"github.com/my/app/internal/transport/http/handlers"
	appmiddleware "github.com/my/app/internal/transport/http/middleware"
)

type Options struct {
	Log            *zap.Logger
	SwaggerEnabled bool
	JWTSecret      string
	RequireAuth    bool
	Reports        *handlers.ReportsHandler
	Inbound        *handlers.InboundHandler
	Feedback       *handlers.FeedbackHandler
	Review         *handlers.ReviewHandler
	Budget         appmiddleware.BudgetPolicy
}

func Register(e *echo.Echo, reply *handlers.ReplyHandler, opts Options) {
	log := opts.Log
	if log == nil {
		log = zap.NewNop()
	}

	e.Use(appmiddleware.RequestID())
	e.Use(appmiddleware.Logger(log))
	e.Use(appmiddleware.Recover())

	e.GET("/healthz", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	if opts.JWTSecret != "" {
		log.Info("service JWT verification enabled for /v1")
	} else {
		log.Warn("service JWT disabled: trusting X-Tenant-Id header (dev mode)")
	}

	publicV1 := e.Group("/v1")
	if opts.Budget.Fallback > 0 {
		publicV1.Use(appmiddleware.Deadline(opts.Budget))
	}
	if opts.Inbound != nil {
		publicV1.POST("/inbound/:channel", opts.Inbound.Receive)
	}

	protectedV1 := e.Group("/v1")
	if opts.Budget.Fallback > 0 {
		protectedV1.Use(appmiddleware.Deadline(opts.Budget))
	}
	protectedV1.Use(appmiddleware.RequireCompany(opts.JWTSecret, opts.RequireAuth))
	protectedV1.POST("/reply", reply.Create, appmiddleware.RequirePermission("ai:reply:create"))

	if opts.Feedback != nil {
		protectedV1.POST("/reply/feedback", opts.Feedback.Create, appmiddleware.RequirePermission("ai:feedback:create"))
	}

	if opts.Review != nil {
		protectedV1.POST("/admin/review-queue/:id/approve", opts.Review.Approve, appmiddleware.RequirePermission("ai:review:approve"))
		protectedV1.POST("/admin/review-queue/:id/reject", opts.Review.Reject, appmiddleware.RequirePermission("ai:review:reject"))
	}

	if opts.Reports != nil {
		protectedV1.GET("/reports/ai-accuracy", opts.Reports.AIAccuracy, appmiddleware.RequirePermission("ai:reports:read"))
		protectedV1.GET("/reports/answer-rate", opts.Reports.AnswerRate, appmiddleware.RequirePermission("ai:reports:read"))
		protectedV1.GET("/reports/knowledge-gaps", opts.Reports.KnowledgeGaps, appmiddleware.RequirePermission("ai:reports:read"))
	}

	if opts.SwaggerEnabled {
		e.GET("/swagger/*", echoSwagger.WrapHandler)
		registerAPIDocs(e)
	}
}
