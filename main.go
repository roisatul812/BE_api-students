package main

import (
	"context"
	"log"

	"api-students/database"
	"api-students/repository"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/joho/godotenv"
)

var studentRepository repository.StudentRepository

func main() {
	// Memuat konfigurasi dari file .env
	if err := godotenv.Load(); err != nil {
		log.Println("Peringatan: file .env tidak ditemukan")
	}

	// Context untuk koneksi database
	ctx := context.Background()

	// Membuat koneksi ke PostgreSQL
	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatal("Gagal terhubung ke database:", err)
	}
	defer db.Close()

	// Menghubungkan repository dengan database
	studentRepository = repository.NewPostgresStudentRepository(db)

	// Membuat aplikasi Fiber
	app := fiber.New()

	// Middleware
	app.Use(requestid.New())
	app.Use(logger.New())

	// Endpoint health check
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

	// Route API
	api := app.Group("/api/v1")
	studentsAPI := api.Group("/students")

	studentsAPI.Get("/", listStudents)
	studentsAPI.Get("/:id", getStudent)
	studentsAPI.Post("/", createStudent)
	studentsAPI.Put("/:id", replaceStudent)
	studentsAPI.Patch("/:id", patchStudent)
	studentsAPI.Delete("/:id", deleteStudent)

	log.Println("Server berjalan di http://localhost:3000")
	log.Fatal(app.Listen(":3000"))
}