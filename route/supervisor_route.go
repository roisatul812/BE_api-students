package route

import (
	"api-students/app/service"
	"api-students/helper"
	"api-students/middleware"

	"github.com/gofiber/fiber/v2"
)

func SupervisorRoutes(
	app *fiber.App,
	supervisorService *service.SupervisorService,
	jwtManager *helper.JWTManager,
	permissionSet *helper.PermissionSet,
) {
	api := app.Group("/api/v1")

	supervisors := api.Group("/supervisors",
		middleware.RequireJSON,
		middleware.RequireAuth(jwtManager),
	)

	supervisors.Get("/", supervisorService.List)
	supervisors.Get("/:id", supervisorService.Get)
	supervisors.Post("/:id/assign", middleware.RequirePermission(permissionSet, "supervision:manage"), supervisorService.Assign)
	supervisors.Patch("/:id", middleware.RequirePermission(permissionSet, "supervision:manage"), supervisorService.UpdateQuota)

	supervisions := api.Group("/supervisions",
		middleware.RequireJSON,
		middleware.RequireAuth(jwtManager),
	)

	supervisions.Post("/switch", middleware.RequirePermission(permissionSet, "supervision:manage"), supervisorService.Switch)
}
