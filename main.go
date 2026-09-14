package main

import (
	"context"
	"log"
	"os"

	"api-students/app/handler"
	"api-students/app/repository"
	"api-students/app/service"
	"api-students/config"
	"api-students/database"
	"api-students/helper"
	"api-students/route"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("file .env tidak ditemukan, menggunakan environment variable")
	}

	authConfig := config.LoadAuthConfig()

	if authConfig.JWTSecret == "" {
		log.Fatal("JWT_SECRET wajib diisi")
	}

	ctx := context.Background()

	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Student dependencies
	studentRepository := repository.NewPostgresStudentRepository(db)
	studentService := service.NewStudentService(studentRepository)
	studentHandler := handler.NewStudentHandler(studentService)

	// Authentication dependencies
	userRepository := repository.NewUserRepository(db)
	tokenRepository := repository.NewTokenRepository(db)

	jwtManager := helper.NewJWTManager(
		authConfig.JWTSecret,
		authConfig.JWTIssuer,
		authConfig.JWTAccessTTL,
	)

	authService := service.NewAuthService(
		userRepository,
		tokenRepository,
		jwtManager,
		authConfig.JWTRefreshTTL,
	)

	authHandler := handler.NewAuthHandler(authService)

	app := fiber.New()

	app.Use(requestid.New())
	app.Use(logger.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})

	route.StudentRoutes(app, studentHandler, jwtManager)
	route.AuthRoutes(app, authHandler, jwtManager)

	log.Println("server berjalan di http://localhost:3000")

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Fatal(app.Listen(":" + port))
}
