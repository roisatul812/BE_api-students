package helper

import (
	"context"
	"time"

	"api-students/app/model"

	"github.com/gofiber/fiber/v2"
)

const LocalsAuthUser = "authUser"

func CurrentUser(c *fiber.Ctx) (model.AuthUser, bool) {
	user, ok := c.Locals(LocalsAuthUser).(model.AuthUser)
	return user, ok
}

func RequestID(c *fiber.Ctx) string {
	requestID, _ := c.Locals("requestid").(string)
	return requestID
}

// RequestContext membuat context dengan timeout 5 detik untuk mengamankan
// operasi database dari request yang menggantung terlalu lama.
func RequestContext(c *fiber.Ctx) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Context(), 5*time.Second)
}
