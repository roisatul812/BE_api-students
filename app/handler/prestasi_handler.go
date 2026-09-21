package handler

import (
	"api-students/app/model"
	"api-students/app/service"

	"github.com/gofiber/fiber/v2"
)

type PrestasiHandler struct {
	service *service.PrestasiService
}

func NewPrestasiHandler(prestasiService *service.PrestasiService) *PrestasiHandler {
	return &PrestasiHandler{
		service: prestasiService,
	}
}

func (h *PrestasiHandler) FindByNIM(c *fiber.Ctx) error {
	nim := c.Params("nim")

	if nim == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "NIM tidak boleh kosong",
		})
	}

	prestasi, err := h.service.FindByNIM(c.Context(), nim)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "gagal mengambil data prestasi",
		})
	}

	if prestasi == nil {
		prestasi = []model.Prestasi{}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": prestasi,
	})
}
