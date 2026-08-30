package models

import "time"

type Student struct {
	ID       int       `json:"id"`
	NIM      string    `json:"nim"`
	Name     string    `json:"name"`
	Grade    float64   `json:"grade"`
	IsActive bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// Request untuk POST
type CreateStudentRequest struct {
	NIM      string  `json:"nim"`
	Name     string  `json:"name"`
	Grade    float64 `json:"grade"`
	IsActive bool    `json:"is_active"`
}

// Request untuk PUT
// Semua field wajib dikirim.
type ReplaceStudentRequest struct {
	NIM      string  `json:"nim"`
	Name     string  `json:"name"`
	Grade    float64 `json:"grade"`
	IsActive bool    `json:"is_active"`
}

// Request untuk PATCH
// Pointer digunakan agar bisa membedakan
// field yang tidak dikirim dengan field bernilai kosong/false/0.
type PatchStudentRequest struct {
	NIM      *string  `json:"nim"`
	Name     *string  `json:"name"`
	Grade    *float64 `json:"grade"`
	IsActive *bool    `json:"is_active"`
}