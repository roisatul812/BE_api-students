package helper

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func ParseInt(value string, defaultValue int) int {
	var result int

	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}

	return result
}

// ParamID membaca parameter :id dari path dan memastikan nilainya berupa
// bilangan bulat positif (> 0).
func ParamID(c *fiber.Ctx) (int, bool) {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
