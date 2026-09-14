package handler

import (
	"errors"
	"strconv"
	"strings"

	"api-students/app/model"
	"api-students/app/service"
	"api-students/helper"

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

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "user tidak terautentikasi",
		})
	}

	student, err := h.service.FindByID(c.Context(), id, currentUser)
	if err != nil {
		var notFoundErr *service.NotFoundError
		if errors.As(err, &notFoundErr) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": notFoundErr.Message,
			})
		}

		var forbiddenErr *service.ForbiddenError
		if errors.As(err, &forbiddenErr) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"message": forbiddenErr.Message,
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

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "user tidak terautentikasi",
		})
	}

	var req model.CreateStudentRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "JSON tidak valid",
		})
	}

	student, err := h.service.Create(
		c.Context(),
		req,
		currentUser.UserID,
	)
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

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		c.Set("WWW-Authenticate", `Bearer realm="api"`)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "autentikasi diperlukan",
		})
	}

	student, err := h.service.Replace(c.Context(), id, req, currentUser)
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

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		c.Set("WWW-Authenticate", `Bearer realm="api"`)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "autentikasi diperlukan",
		})
	}
	student, err := h.service.Patch(c.Context(), id, req, currentUser)
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

	var forbiddenErr *service.ForbiddenError
	if errors.As(err, &forbiddenErr) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": forbiddenErr.Message,
		})
	}

	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"message": "terjadi kesalahan pada server",
	})
}
