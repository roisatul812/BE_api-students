package main

import (
	"fmt"

	"api-students/models"

	"github.com/gofiber/fiber/v2"
)

type WebResponse struct {
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

type ErrorResponse struct {
	Message string      `json:"message"`
	Errors  interface{} `json:"errors,omitempty"`
}

type Meta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func success(c *fiber.Ctx, status int, data interface{}) error {
	return c.Status(status).JSON(WebResponse{
		Data: data,
	})
}

func created(c *fiber.Ctx, data interface{}) error {
	student, ok := data.(models.Student)
	if ok {
		c.Location(fmt.Sprintf("/api/v1/students/%d", student.ID))
	}

	return c.Status(fiber.StatusCreated).JSON(WebResponse{
		Data: data,
	})
}

func noContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

func fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(ErrorResponse{
		Message: message,
	})
}

func failValidation(c *fiber.Ctx, errors map[string]string) error {
	return c.Status(fiber.StatusUnprocessableEntity).JSON(ErrorResponse{
		Message: "validasi gagal",
		Errors:  errors,
	})
}

func parseInt(value string, defaultValue int) int {
	var result int

	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}

	return result
}
