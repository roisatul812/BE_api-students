package model

import "time"

type Student struct {
	ID        int       `json:"id"`
	NIM       string    `json:"nim"`
	Name      string    `json:"name"`
	Grade     int       `json:"grade"`
	IsActive  bool      `json:"is_active"`
	OwnerID   int       `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
}

// Request untuk POST
type CreateStudentRequest struct {
	NIM      string  `json:"nim" validate:"required,nim"`
	Name     string  `json:"name" validate:"required"`
	Grade    float64 `json:"grade" validate:"min=0,max=100"`
	IsActive bool    `json:"is_active"`
}

// Request untuk PUT
// Semua field wajib dikirim.
type ReplaceStudentRequest struct {
	NIM      string  `json:"nim" validate:"required,nim"`
	Name     string  `json:"name" validate:"required"`
	Grade    float64 `json:"grade" validate:"min=0,max=100"`
	IsActive bool    `json:"is_active"`
}

// Request untuk PATCH
// Pointer digunakan agar bisa membedakan
// field yang tidak dikirim dengan field bernilai kosong/false/0.
type PatchStudentRequest struct {
	NIM      *string  `json:"nim" validate:"omitnil,nim"`
	Name     *string  `json:"name" validate:"omitnil,min=1"`
	Grade    *float64 `json:"grade" validate:"omitnil,min=0,max=100"`
	IsActive *bool    `json:"is_active" validate:"omitnil"`
}

type CursorParam struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int       `json:"id"`
}

type CursorMeta struct {
	Limit      int     `json:"limit"`
	NextCursor *string `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more"`
}

type StudentCursorResponse struct {
	Data []Student  `json:"data"`
	Meta CursorMeta `json:"meta"`
}
