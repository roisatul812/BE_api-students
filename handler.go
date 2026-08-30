package main

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

var students = []Student{
	{
		ID:        1,
		NIM:       "240001",
		Name:      "Isa",
		Grade:     85,
		IsActive:  true,
		CreatedAt: time.Now(),
	},
	{
		ID:        2,
		NIM:       "240002",
		Name:      "Sari",
		Grade:     90,
		IsActive:  true,
		CreatedAt: time.Now(),
	},
	{
		ID:        3,
		NIM:       "240003",
		Name:      "Budi",
		Grade:     78,
		IsActive:  false,
		CreatedAt: time.Now(),
	},
}

var nextStudentID = 4

// GET /api/v1/students
func listStudents(c *fiber.Ctx) error {
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

	// Filter data
	filtered := make([]Student, 0)

	for _, student := range students {

		// Search berdasarkan nama
		if search != "" &&
			!strings.Contains(
				strings.ToLower(student.Name),
				strings.ToLower(search),
			) {
			continue
		}

		// Filter is_active
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

	// Whitelist sorting
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

	// Sorting
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

	// Pagination
	total := len(filtered)

	totalPages := 0

	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	start := (page - 1) * limit

	if start >= total {
		return success(c, fiber.StatusOK, struct {
			Items []Student `json:"items"`
			Meta  Meta      `json:"meta"`
		}{
			Items: []Student{},
			Meta: Meta{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: totalPages,
			},
		})
	}

	end := start + limit

	if end > total {
		end = total
	}

	result := filtered[start:end]

	return success(c, fiber.StatusOK, struct {
		Items []Student `json:"items"`
		Meta  Meta      `json:"meta"`
	}{
		Items: result,
		Meta: Meta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// GET /api/v1/students/:id
func getStudent(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))

	if err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"ID harus berupa angka",
		)
	}

	for _, student := range students {
		if student.ID == id {
			return success(
				c,
				fiber.StatusOK,
				student,
			)
		}
	}

	return fail(
		c,
		fiber.StatusNotFound,
		"student tidak ditemukan",
	)
}

// POST /api/v1/students
func createStudent(c *fiber.Ctx) error {
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

	var request CreateStudentRequest

	if err := json.Unmarshal(
		c.Body(),
		&request,
	); err != nil {
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

	if isNIMExists(request.NIM, 0) {
		return fail(
			c,
			fiber.StatusConflict,
			"NIM sudah digunakan",
		)
	}

	student := Student{
		ID:        nextStudentID,
		NIM:       request.NIM,
		Name:      request.Name,
		Grade:     request.Grade,
		IsActive:  request.IsActive,
		CreatedAt: time.Now(),
	}

	students = append(students, student)

	nextStudentID++

	return created(c, student)
}

// PUT /api/v1/students/:id
func replaceStudent(c *fiber.Ctx) error {
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

	index := findStudentIndex(id)

	if index == -1 {
		return fail(
			c,
			fiber.StatusNotFound,
			"student tidak ditemukan",
		)
	}

	// Cek field wajib pada PUT
	var raw map[string]json.RawMessage

	if err := json.Unmarshal(
		c.Body(),
		&raw,
	); err != nil {
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

	var request ReplaceStudentRequest

	if err := json.Unmarshal(
		c.Body(),
		&request,
	); err != nil {
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

	if isNIMExists(request.NIM, id) {
		return fail(
			c,
			fiber.StatusConflict,
			"NIM sudah digunakan",
		)
	}

	students[index].NIM = request.NIM
	students[index].Name = request.Name
	students[index].Grade = request.Grade
	students[index].IsActive = request.IsActive

	return success(
		c,
		fiber.StatusOK,
		students[index],
	)
}

// PATCH /api/v1/students/:id
func patchStudent(c *fiber.Ctx) error {
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

	index := findStudentIndex(id)

	if index == -1 {
		return fail(
			c,
			fiber.StatusNotFound,
			"student tidak ditemukan",
		)
	}

	var request PatchStudentRequest

	if err := json.Unmarshal(
		c.Body(),
		&request,
	); err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"JSON tidak valid",
		)
	}

	// Pastikan minimal ada satu field
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

		if isNIMExists(*request.NIM, id) {
			return fail(
				c,
				fiber.StatusConflict,
				"NIM sudah digunakan",
			)
		}

		students[index].NIM = *request.NIM
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

		students[index].Name = *request.Name
	}

	if request.Grade != nil {
		if *request.Grade < 0 ||
			*request.Grade > 100 {

			return failValidation(
				c,
				map[string]string{
					"grade": "grade harus berada di antara 0 dan 100",
				},
			)
		}

		students[index].Grade = *request.Grade
	}

	if request.IsActive != nil {
		students[index].IsActive = *request.IsActive
	}

	return success(
		c,
		fiber.StatusOK,
		students[index],
	)
}

// DELETE /api/v1/students/:id
func deleteStudent(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))

	if err != nil {
		return fail(
			c,
			fiber.StatusBadRequest,
			"ID harus berupa angka",
		)
	}

	index := findStudentIndex(id)

	if index == -1 {
		return fail(
			c,
			fiber.StatusNotFound,
			"student tidak ditemukan",
		)
	}

	students = append(
		students[:index],
		students[index+1:]...,
	)

	return noContent(c)
}

// Validasi data Student
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
		errors["grade"] =
			"grade harus berada di antara 0 dan 100"
	}

	return errors
}

// Mengecek apakah NIM sudah digunakan
func isNIMExists(
	nim string,
	exceptID int,
) bool {

	for _, student := range students {
		if student.NIM == nim &&
			student.ID != exceptID {
			return true
		}
	}

	return false
}

// Mencari index Student berdasarkan ID
func findStudentIndex(id int) int {
	for i, student := range students {
		if student.ID == id {
			return i
		}
	}

	return -1
}