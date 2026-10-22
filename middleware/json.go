package middleware

import (
	"strings"

	"api-students/helper"

	"github.com/gofiber/fiber/v2"
)

// RequireJSON memastikan request dengan body memiliki header Content-Type application/json.
func RequireJSON(c *fiber.Ctx) error {
	if c.Method() == fiber.MethodPost || c.Method() == fiber.MethodPut || c.Method() == fiber.MethodPatch {
		if len(c.Body()) > 0 {
			contentType := c.Get(fiber.HeaderContentType)
			if !strings.HasPrefix(contentType, fiber.MIMEApplicationJSON) {
				return helper.UnsupportedMediaType("Content-Type harus application/json")
			}
		}
	}
	return c.Next()
}
