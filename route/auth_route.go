package route

import (
	"api-students/app/handler"
	"api-students/helper"
	"api-students/middleware"

	"github.com/gofiber/fiber/v2"
)

func AuthRoutes(
	app *fiber.App,
	authHandler *handler.AuthHandler,
	jwtManager *helper.JWTManager,
) {
	api := app.Group("/api/v1")
	auth := api.Group("/auth")

	auth.Post("/register", authHandler.Register)
	auth.Post("/login", middleware.LoginRateLimiter(), authHandler.Login)
	auth.Post("/refresh", authHandler.Refresh)
	auth.Post("/logout", authHandler.Logout)
	auth.Get("/me", middleware.RequireAuth(jwtManager), authHandler.Me)
}
