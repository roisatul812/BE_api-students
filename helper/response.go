package helper

import (
	"fmt"

	model "api-students/app/model"

	"github.com/gofiber/fiber/v2"
)

type WebResponse struct {
	Data    interface{} `json:"data,omitempty"`
	Message *string     `json:"message,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

type ErrorResponse struct {
	Message *string     `json:"message"`
	Errors  interface{} `json:"errors,omitempty"`
}

func Success(c *fiber.Ctx, status int, data interface{}) error {
	return c.Status(status).JSON(WebResponse{
		Data: data,
	})
}

func Created(c *fiber.Ctx, data interface{}) error {
	student, ok := data.(model.Student)

	if ok {
		c.Location(fmt.Sprintf("/api/v1/students/%d", student.ID))
	}

	return c.Status(fiber.StatusCreated).JSON(WebResponse{
		Data: data,
	})
}

func NoContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

func Fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(ErrorResponse{
		Message: &message,
	})
}

func FailValidation(c *fiber.Ctx, errors map[string]string) error {
	message := "validasi gagal"

	return c.Status(fiber.StatusUnprocessableEntity).JSON(ErrorResponse{
		Message: &message,
		Errors:  errors,
	})
}
