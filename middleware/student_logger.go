package middleware

import (
	"log"

	"api-students/helper"

	"github.com/gofiber/fiber/v2"
)

func StudentRequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authUser, ok := helper.CurrentUser(c)

		if ok {
			log.Printf(
				"students request user_id=%d role=%s method=%s path=%s",
				authUser.UserID,
				authUser.Role,
				c.Method(),
				c.OriginalURL(),
			)
		}

		return c.Next()
	}
}
