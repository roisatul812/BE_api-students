package route

import (
	"api-students/app/service"
	"api-students/helper"
	"api-students/middleware"

	"github.com/gofiber/fiber/v2"
)

func PortfolioRoutes(
	app *fiber.App,
	portfolioService *service.PortfolioService,
	jwtManager *helper.JWTManager,
) {
	api := app.Group("/api/v1")

	// --- student portfolios ---
	students := api.Group("/students",
		middleware.RequireJSON,
		middleware.RequireAuth(jwtManager),
	)
	students.Get("/:id/portfolio", portfolioService.Get)
	students.Put("/:id/portfolio", portfolioService.Upsert)
	students.Post("/:id/portfolio/projects", portfolioService.AddProject)
	students.Delete("/:id/portfolio/projects/:projectId", portfolioService.DeleteProject)

	// --- portfolio statistics ---
	portfolios := api.Group("/portfolios",
		middleware.RequireJSON,
		middleware.RequireAuth(jwtManager),
	)
	portfolios.Get("/stats/technologies", portfolioService.GetTechnologyStats)
}
