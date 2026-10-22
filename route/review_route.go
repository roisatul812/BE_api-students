package route

import (
	"api-students/app/service"
	"api-students/helper"
	"api-students/middleware"

	"github.com/gofiber/fiber/v2"
)

func ReviewRoutes(
	app *fiber.App,
	reviewService *service.ReviewService,
	jwtManager *helper.JWTManager,
) {
	api := app.Group("/api/v1")

	// --- ulasan kelas (Courses Reviews) ---
	courses := api.Group("/courses",
		middleware.RequireJSON,
		middleware.RequireAuth(jwtManager),
	)
	courses.Get("/:id/reviews", reviewService.ListByCourse)
	courses.Get("/:id/reviews/summary", reviewService.Summary)
	courses.Post("/:id/reviews", reviewService.Create)

	// --- ulasan kelas (MongoDB) ---
	reviews := api.Group("/reviews",
		middleware.RequireJSON,
		middleware.RequireAuth(jwtManager),
	)
	reviews.Get("/:id", reviewService.Get)
	reviews.Patch("/:id", reviewService.Update)
	reviews.Delete("/:id", reviewService.Delete)
	reviews.Post("/:id/replies", reviewService.Reply)
	reviews.Post("/:id/helpful", reviewService.MarkHelpful)
}
