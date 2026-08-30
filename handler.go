package main

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"api-students/models"

	"github.com/gofiber/fiber/v2"
)

// GET /api/v1/students
func listStudents(c *fiber.Ctx) error {
	ctx := c.UserContext()

	page := parseInt(c.Query("page", "1"), 1)
	limit := parseInt(c.Query("limit", "10"), 10)

	if page < 1 {
		page = 1
	}

	if limit < 1 {
		limit = 10
	}

	if limit > 100 {
		limit = 100
	}

	search := strings.TrimSpace(c.Query("search"))
	sortField := c.Query("sort", "id")
	order := strings.ToLower(c.Query("order", "asc"))
	activeFilter := c.Query("is_active")

	allowedSort := map[string]bool{
		"id":         true,
		"nim":        true,
		"name":       true,
		"grade":      true,
		"is_active":  true,
		"created_at": true,
	}

	if !allowedSort[sortField] {
		return fail(
			c,
			fiber.StatusBadRequest,
			"field sorting tidak diperbolehkan",
		)
	}

	if order != "asc" && order != "desc" {
		return fail(
			c,
			fiber.StatusBadRequest,
			"order harus asc atau desc",
		)
	}

	students, err := studentRepository.FindAll(ctx)
	if err != nil {
		return fail(
			c,
			fiber.StatusInternalServerError,
			"gagal mengambil data student",
		)
	}

	filtered := make([]models.Student, 0)

	for _, student := range students {
		if search != "" &&
			!strings.Contains(
				strings.ToLower(student.Name),
				strings.ToLower(search),
			) {
			continue
		}

		if activeFilter != "" {
			active, err := strconv.ParseBool(activeFilter)

			if err != nil {
				return fail(
					c,
					fiber.StatusBadRequest,
					"is_active harus bernilai true atau false",
				)
			}

			if student.IsActive != active {
				continue
			}
		}

		filtered = append(filtered, student)
	}

	sort.Slice(filtered, func(i, j int) bool {
		switch sortField {
		case "id":
			if order == "desc" {
				return filtered[i].ID > filtered[j].ID
			}
			return filtered[i].ID < filtered[j].ID

		case "nim":
			if order == "desc" {
				return filtered[i].NIM > filtered[j].NIM
			}
			return filtered[i].NIM < filtered[j].NIM

		case "name":
			if order == "desc" {
				return filtered[i].Name > filtered[j].Name
			}
			return filtered[i].Name < filtered[j].Name

		case "grade":
			if order == "desc" {
				return filtered[i].Grade > filtered[j].Grade
			}
			return filtered[i].Grade < filtered[j].Grade

		case "is_active":
			if order == "desc" {
				return filtered[i].IsActive && !filtered[j].IsActive
			}
			return !filtered[i].IsActive && filtered[j].IsActive

		case "created_at":
			if order == "desc" {
				return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
			}
			return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
		}

		return false
	})

	total := len(filtered)

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	start := (page - 1) * limit

	if start >= total {
		return success(
			c,
			fiber.StatusOK,
			struct {
				Items []models.Student `json:"items"`
				Meta  Meta             `json:"meta"`
			}{
				Items: []models.Student{},
				Meta: Meta{
					Page:       page,
					Limit:      limit,
					Total:      total,
					TotalPages: totalPages,
				},
			},
		)
	}

	end := start + limit

	if end > total {
		end = total
	}

	result := filtered[start:end]

	return success(
		c,
		fiber.StatusOK,
		struct {
			Items []models.Student `json:"items"`
			Meta  Meta             `json:"meta"`
		}{
			Items: result,
			Meta: Meta{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: totalPages,
			},
		},
	)
}

// GET /api/v1/students/:id
func getStudent(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"ID harus berupa angka",
		)
	}

	student, err := studentRepository.FindByID(ctx, id)
	if err != nil {
		return fail(
			c,
			fiber.StatusNotFound,
			"student tidak ditemukan",
		)
	}

	return success(
		c,
		fiber.StatusOK,
		student,
	)
}

// POST /api/v1/students
func createStudent(c *fiber.Ctx) error {
	ctx := c.UserContext()

	if !strings.Contains(
		c.Get("Content-Type"),
		"application/json",
	) {
		return fail(
			c,
			fiber.StatusUnsupportedMediaType,
			"Content-Type harus application/json",
		)
	}

	var request models.CreateStudentRequest

	if err := json.Unmarshal(c.Body(), &request); err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"JSON tidak valid",
		)
	}

	errors := validateStudent(
		request.NIM,
		request.Name,
		request.Grade,
	)

	if len(errors) > 0 {
		return failValidation(c, errors)
	}

	if isNIMExists(ctx, request.NIM, 0) {
		return fail(
			c,
			fiber.StatusConflict,
			"NIM sudah digunakan",
		)
	}

	student := models.Student{
		NIM:      request.NIM,
		Name:     request.Name,
		Grade:    request.Grade,
		IsActive: request.IsActive,
	}

	err := studentRepository.Create(ctx, &student)
	if err != nil {
		return fail(
			c,
			fiber.StatusInternalServerError,
			"gagal membuat student",
		)
	}

	return created(c, student)
}

// PUT /api/v1/students/:id
func replaceStudent(c *fiber.Ctx) error {
	ctx := c.UserContext()

	if !strings.Contains(
		c.Get("Content-Type"),
		"application/json",
	) {
		return fail(
			c,
			fiber.StatusUnsupportedMediaType,
			"Content-Type harus application/json",
		)
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"ID harus berupa angka",
		)
	}

	_, err = studentRepository.FindByID(ctx, id)
	if err != nil {
		return fail(
			c,
			fiber.StatusNotFound,
			"student tidak ditemukan",
		)
	}

	var raw map[string]json.RawMessage

	if err := json.Unmarshal(c.Body(), &raw); err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"JSON tidak valid",
		)
	}

	requiredFields := []string{
		"nim",
		"name",
		"grade",
		"is_active",
	}

	for _, field := range requiredFields {
		if _, exists := raw[field]; !exists {
			return failValidation(
				c,
				map[string]string{
					field: "field wajib dikirim",
				},
			)
		}
	}

	var request models.ReplaceStudentRequest

	if err := json.Unmarshal(c.Body(), &request); err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"JSON tidak valid",
		)
	}

	errors := validateStudent(
		request.NIM,
		request.Name,
		request.Grade,
	)

	if len(errors) > 0 {
		return failValidation(c, errors)
	}

	if isNIMExists(ctx, request.NIM, id) {
		return fail(
			c,
			fiber.StatusConflict,
			"NIM sudah digunakan",
		)
	}

	student := models.Student{
		ID:       id,
		NIM:      request.NIM,
		Name:     request.Name,
		Grade:    request.Grade,
		IsActive: request.IsActive,
	}

	err = studentRepository.Update(ctx, &student)
	if err != nil {
		return fail(
			c,
			fiber.StatusInternalServerError,
			"gagal mengubah student",
		)
	}

	updatedStudent, err := studentRepository.FindByID(ctx, id)
	if err != nil {
		return fail(
			c,
			fiber.StatusInternalServerError,
			"gagal mengambil data student",
		)
	}

	return success(
		c,
		fiber.StatusOK,
		updatedStudent,
	)
}

// PATCH /api/v1/students/:id
func patchStudent(c *fiber.Ctx) error {
	ctx := c.UserContext()

	if !strings.Contains(
		c.Get("Content-Type"),
		"application/json",
	) {
		return fail(
			c,
			fiber.StatusUnsupportedMediaType,
			"Content-Type harus application/json",
		)
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"ID harus berupa angka",
		)
	}

	student, err := studentRepository.FindByID(ctx, id)
	if err != nil {
		return fail(
			c,
			fiber.StatusNotFound,
			"student tidak ditemukan",
		)
	}

	var request models.PatchStudentRequest

	if err := json.Unmarshal(c.Body(), &request); err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"JSON tidak valid",
		)
	}

	if request.NIM == nil &&
		request.Name == nil &&
		request.Grade == nil &&
		request.IsActive == nil {

		return failValidation(
			c,
			map[string]string{
				"body": "minimal satu field harus dikirim",
			},
		)
	}

	if request.NIM != nil {
		if strings.TrimSpace(*request.NIM) == "" {
			return failValidation(
				c,
				map[string]string{
					"nim": "NIM tidak boleh kosong",
				},
			)
		}

		if isNIMExists(ctx, *request.NIM, id) {
			return fail(
				c,
				fiber.StatusConflict,
				"NIM sudah digunakan",
			)
		}

		student.NIM = *request.NIM
	}

	if request.Name != nil {
		if strings.TrimSpace(*request.Name) == "" {
			return failValidation(
				c,
				map[string]string{
					"name": "nama tidak boleh kosong",
				},
			)
		}

		student.Name = *request.Name
	}

	if request.Grade != nil {
		if *request.Grade < 0 || *request.Grade > 100 {
			return failValidation(
				c,
				map[string]string{
					"grade": "grade harus berada di antara 0 dan 100",
				},
			)
		}

		student.Grade = *request.Grade
	}

	if request.IsActive != nil {
		student.IsActive = *request.IsActive
	}

	err = studentRepository.Update(ctx, student)
	if err != nil {
		return fail(
			c,
			fiber.StatusInternalServerError,
			"gagal mengubah student",
		)
	}

	updatedStudent, err := studentRepository.FindByID(ctx, id)
	if err != nil {
		return fail(
			c,
			fiber.StatusInternalServerError,
			"gagal mengambil data student",
		)
	}

	return success(
		c,
		fiber.StatusOK,
		updatedStudent,
	)
}

// DELETE /api/v1/students/:id
func deleteStudent(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"ID harus berupa angka",
		)
	}

	_, err = studentRepository.FindByID(ctx, id)
	if err != nil {
		return fail(
			c,
			fiber.StatusNotFound,
			"student tidak ditemukan",
		)
	}

	err = studentRepository.Delete(ctx, id)
	if err != nil {
		return fail(
			c,
			fiber.StatusInternalServerError,
			"gagal menghapus student",
		)
	}

	return noContent(c)
}

// Validasi data student
func validateStudent(
	nim string,
	name string,
	grade float64,
) map[string]string {
	errors := make(map[string]string)

	if strings.TrimSpace(nim) == "" {
		errors["nim"] = "NIM tidak boleh kosong"
	}

	if strings.TrimSpace(name) == "" {
		errors["name"] = "nama tidak boleh kosong"
	}

	if grade < 0 || grade > 100 {
		errors["grade"] = "grade harus berada di antara 0 dan 100"
	}

	return errors
}

// Mengecek apakah NIM sudah digunakan
func isNIMExists(
	ctx context.Context,
	nim string,
	exceptID int,
) bool {
	students, err := studentRepository.FindAll(ctx)
	if err != nil {
		return false
	}

	for _, student := range students {
		if student.NIM == nim && student.ID != exceptID {
			return true
		}
	}

	return false
}
