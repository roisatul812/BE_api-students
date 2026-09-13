package main

import (
	"context"
	"log"

	"api-students/app/handler"
	"api-students/app/repository"
	"api-students/app/service"
	"api-students/database"
	"api-students/route"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Peringatan: file .env tidak ditemukan")
	}

	ctx := context.Background()

	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatal("Gagal terhubung ke database:", err)
	}
	defer db.Close()

	studentRepository := repository.NewPostgresStudentRepository(db)
	studentService := service.NewStudentService(studentRepository)
	studentHandler := handler.NewStudentHandler(studentService)

	app := fiber.New()

	app.Use(requestid.New())
	app.Use(logger.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		if err := db.Ping(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status":   "error",
				"database": "unavailable",
			})
		}

		return c.JSON(fiber.Map{
			"status":   "ok",
			"database": "connected",
		})
	})

	route.StudentRoutes(app, studentHandler)

	log.Println("Server berjalan di http://localhost:3000")

	log.Fatal(app.Listen(":3000"))
}
