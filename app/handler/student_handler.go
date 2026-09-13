package handler

import (
	"errors"
	"strconv"
	"strings"

	model "api-students/app/model"
	"api-students/app/service"

	"github.com/gofiber/fiber/v2"
)

type StudentHandler struct {
	service *service.StudentService
}

func NewStudentHandler(studentService *service.StudentService) *StudentHandler {
	return &StudentHandler{
		service: studentService,
	}
}

func (h *StudentHandler) FindAll(c *fiber.Ctx) error {
	students, err := h.service.FindAll(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "gagal mengambil data student",
		})
	}

	if students == nil {
		students = []model.Student{}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": students,
	})
}

func (h *StudentHandler) FindByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "ID harus berupa angka",
		})
	}

	student, err := h.service.FindByID(c.Context(), id)
	if err != nil {
		var notFoundErr *service.NotFoundError
		if errors.As(err, &notFoundErr) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": notFoundErr.Message,
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "gagal mengambil data student",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": student,
	})
}

func (h *StudentHandler) Create(c *fiber.Ctx) error {
	if !isJSON(c) {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{
			"message": "Content-Type harus application/json",
		})
	}

	var req model.CreateStudentRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "JSON tidak valid",
		})
	}

	student, err := h.service.Create(c.Context(), req)
	if err != nil {
		return handleServiceError(c, err)
	}

	c.Location("/api/v1/students/" + strconv.Itoa(student.ID))

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data": student,
	})
}

func (h *StudentHandler) Replace(c *fiber.Ctx) error {
	if !isJSON(c) {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{
			"message": "Content-Type harus application/json",
		})
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "ID harus berupa angka",
		})
	}

	var req model.ReplaceStudentRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "JSON tidak valid",
		})
	}

	student, err := h.service.Replace(c.Context(), id, req)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": student,
	})
}

func (h *StudentHandler) Patch(c *fiber.Ctx) error {
	if !isJSON(c) {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{
			"message": "Content-Type harus application/json",
		})
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "ID harus berupa angka",
		})
	}

	var req model.PatchStudentRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "JSON tidak valid",
		})
	}

	student, err := h.service.Patch(c.Context(), id, req)
	if err != nil {
		return handleServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": student,
	})
}

func (h *StudentHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "ID harus berupa angka",
		})
	}

	if err := h.service.Delete(c.Context(), id); err != nil {
		return handleServiceError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func isJSON(c *fiber.Ctx) bool {
	contentType := strings.ToLower(c.Get("Content-Type"))

	return strings.HasPrefix(contentType, "application/json")
}

func handleServiceError(c *fiber.Ctx, err error) error {
	var validationErr *service.ValidationError
	if errors.As(err, &validationErr) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"message": "validasi gagal",
			"errors":  validationErr.Errors,
		})
	}

	var conflictErr *service.ConflictError
	if errors.As(err, &conflictErr) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"message": conflictErr.Message,
		})
	}

	var notFoundErr *service.NotFoundError
	if errors.As(err, &notFoundErr) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"message": notFoundErr.Message,
		})
	}

	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"message": "terjadi kesalahan pada server",
	})
}
