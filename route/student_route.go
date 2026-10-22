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
	prestasiHandler *handler.PrestasiHandler,
	jwtManager *helper.JWTManager,
	permissionSet *helper.PermissionSet,
) {
	api := app.Group("/api/v1")

	students := api.Group(
		"/students",
		middleware.RequireAuth(jwtManager),
		middleware.StudentRequestLogger(),
	)

	students.Get(
		"/",
		middleware.RequirePermission(permissionSet, "student:list"),
		studentHandler.FindAll,
	)

	students.Get(
		"/:id",
		studentHandler.FindByID,
	)

	students.Get(
		"/:nim/prestasi",
		prestasiHandler.FindByNIM,
	)

	students.Post(
		"/",
		middleware.RequirePermission(permissionSet, "student:create"),
		studentHandler.Create,
	)

	students.Put(
		"/:id",
		studentHandler.Replace,
	)

	students.Patch(
		"/:id",
		studentHandler.Patch,
	)

	students.Delete(
		"/:id",
		middleware.RequirePermission(permissionSet, "student:delete"),
		studentHandler.Delete,
	)
}

