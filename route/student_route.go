package route

import (
	"api-students/app/handler"
	"api-students/helper"
	"api-students/middleware"

	"github.com/gofiber/fiber/v2"
)

func StudentRoutes(
	app *fiber.App,
	studentHandler *handler.StudentHandler,
	jwtManager *helper.JWTManager,
) {
	api := app.Group("/api/v1")

	students := api.Group(
		"/students",
		middleware.RequireAuth(jwtManager),
	)

	students.Get("/", studentHandler.FindAll)
	students.Get("/:id", studentHandler.FindByID)
	students.Post("/", studentHandler.Create)
	students.Put("/:id", studentHandler.Replace)
	students.Patch("/:id", studentHandler.Patch)
	students.Delete("/:id", studentHandler.Delete)
}
