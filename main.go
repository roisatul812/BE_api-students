package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

func main() {
	app := fiber.New()

	// Middleware
	app.Use(requestid.New())
	app.Use(logger.New())

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
