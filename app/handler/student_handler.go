package handler

import (
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
	// Content Negotiation (Modul 7 Langkah 8 & D.4.1)
	mediaType, err := helper.Negotiate(c, helper.FormatJSON, helper.FormatCSV)
	if err != nil {
		return err // Mengembalikan 406 Not Acceptable via Centralized ErrorHandler
	}

	cursor := c.Query("cursor")
	limitStr := c.Query("limit")
	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Jika format CSV diminta:
	if mediaType == helper.FormatCSV {
		students, err := h.service.FindAll(c.Context())
		if err != nil {
			return err
		}
		csvBytes, err := helper.GenerateStudentsCSV(students)
		if err != nil {
			return err
		}
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", "attachment; filename=\"students.csv\"")
		return c.Status(fiber.StatusOK).Send(csvBytes)
	}

	// Default response JSON menggunakan Keyset / Cursor Pagination (Modul 7 D.3)
	resp, err := h.service.FindByCursor(c.Context(), cursor, limit)
	if err != nil {
		return err // Mengembalikan 400 Bad Request via ErrorHandler jika cursor invalid
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

func (h *StudentHandler) FindByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return helper.BadRequest("ID harus berupa angka")
	}

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("user tidak terautentikasi")
	}

	student, err := h.service.FindByID(c.Context(), id, currentUser)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": student,
	})
}

func (h *StudentHandler) Create(c *fiber.Ctx) error {
	if !isJSON(c) {
		return helper.UnsupportedMediaType("Content-Type harus application/json")
	}

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("user tidak terautentikasi")
	}

	var req model.CreateStudentRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("JSON tidak valid")
	}

	student, err := h.service.Create(
		c.Context(),
		req,
		currentUser.UserID,
	)
	if err != nil {
		return err
	}

	c.Location("/api/v1/students/" + strconv.Itoa(student.ID))

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"data": student,
	})
}

func (h *StudentHandler) Replace(c *fiber.Ctx) error {
	if !isJSON(c) {
		return helper.UnsupportedMediaType("Content-Type harus application/json")
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return helper.BadRequest("ID harus berupa angka")
	}

	var req model.ReplaceStudentRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("JSON tidak valid")
	}

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		c.Set("WWW-Authenticate", `Bearer realm="api"`)
		return helper.Unauthorized("autentikasi diperlukan")
	}

	student, err := h.service.Replace(c.Context(), id, req, currentUser)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": student,
	})
}

func (h *StudentHandler) Patch(c *fiber.Ctx) error {
	if !isJSON(c) {
		return helper.UnsupportedMediaType("Content-Type harus application/json")
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return helper.BadRequest("ID harus berupa angka")
	}

	var req model.PatchStudentRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("JSON tidak valid")
	}

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		c.Set("WWW-Authenticate", `Bearer realm="api"`)
		return helper.Unauthorized("autentikasi diperlukan")
	}

	student, err := h.service.Patch(c.Context(), id, req, currentUser)
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"data": student,
	})
}

func (h *StudentHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return helper.BadRequest("ID harus berupa angka")
	}

	currentUser, ok := helper.CurrentUser(c)
	if !ok {
		c.Set("WWW-Authenticate", `Bearer realm="api"`)
		return helper.Unauthorized("autentikasi diperlukan")
	}

	if err := h.service.Delete(c.Context(), &id, currentUser); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func isJSON(c *fiber.Ctx) bool {
	contentType := strings.ToLower(c.Get("Content-Type"))
	return strings.HasPrefix(contentType, "application/json")
}
