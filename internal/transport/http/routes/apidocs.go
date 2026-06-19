package routes

import (
	"net/http"

	"github.com/labstack/echo/v5"

	swaggerdocs "github.com/my/app/docs/swagger"
)

const scalarDocsHTML = `<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta
        name="viewport"
        content="width=device-width, initial-scale=1"
    >
    <title>Centric RAG AI Service</title>
</head>

<body>
    <div id="app"></div>

    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
        Scalar.createApiReference("#app", {
            url: "/openapi.json",
            layout: "modern",
            theme: "default",
            showSidebar: true,
            hideModels: false,
            persistAuth: true,
            agent: {
                disabled: true
            }
        })
    </script>
</body>
</html>`

func registerAPIDocs(e *echo.Echo) {
	e.GET("/openapi.json", func(c *echo.Context) error {
		return c.Blob(
			http.StatusOK,
			"application/json; charset=utf-8",
			[]byte(swaggerdocs.SwaggerInfo.ReadDoc()),
		)
	})

	e.GET("/docs", func(c *echo.Context) error {
		return c.HTML(http.StatusOK, scalarDocsHTML)
	})

	e.GET("/docs/", func(c *echo.Context) error {
		return c.Redirect(http.StatusTemporaryRedirect, "/docs")
	})
}
