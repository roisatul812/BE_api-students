package route

import (
	"api-students/app/service"
	"api-students/helper"
	"api-students/middleware"

	"github.com/gofiber/fiber/v2"
)

func CourseRoutes(
	app *fiber.App,
	courseService *service.CourseService,
	jwtManager *helper.JWTManager,
	permissionSet *helper.PermissionSet,
) {
	api := app.Group("/api/v1")

	// --- kelas dan KRS ---
	courses := api.Group("/courses",
		middleware.RequireJSON,
		middleware.RequireAuth(jwtManager),
	)

	// "/mine" didaftarkan SEBELUM "/:id". Fiber mencocokkan route sesuai
	// urutan pendaftaran; bila dibalik, "mine" akan dianggap sebagai :id.
	courses.Get("/", courseService.List)
	courses.Get("/mine", courseService.Mine)
	courses.Get("/:id", courseService.Get)
	courses.Post("/:id/enroll", courseService.Enroll)
	courses.Delete("/:id/enroll", courseService.Drop)
	courses.Patch("/:id", middleware.RequirePermission(permissionSet, "course:manage"), courseService.UpdateQuota)

	enrollments := api.Group("/enrollments",
		middleware.RequireJSON,
		middleware.RequireAuth(jwtManager),
	)
	enrollments.Post("/swap", courseService.Swap)
}
