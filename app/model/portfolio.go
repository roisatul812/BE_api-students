package model

import "time"

// Portfolio adalah representasi portofolio mahasiswa pada level domain.
// Sesuai aturan arsitektur project, package model tidak mengimpor driver MongoDB/BSON.
type Portfolio struct {
	ID           string        `json:"id"`
	StudentID    int           `json:"student_id"`
	Headline     string        `json:"headline"`
	Bio          string        `json:"bio"`
	Projects     []Project     `json:"projects"`
	Certificates []Certificate `json:"certificates"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// Project disimpan secara embedded di dalam dokumen Portfolio.
type Project struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Technologies []string `json:"technologies"`
	Link         string   `json:"link"`
	Year         int      `json:"year"`
}

// Certificate disimpan secara embedded di dalam dokumen Portfolio.
type Certificate struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Issuer string `json:"issuer"`
	Date   string `json:"date"`
}

// UpsertPortfolioRequest digunakan untuk PUT /students/:id/portfolio
type UpsertPortfolioRequest struct {
	Headline     string               `json:"headline" validate:"required,min=5,max=150"`
	Bio          string               `json:"bio" validate:"required,min=10,max=2000"`
	Projects     []CreateProjectInput `json:"projects" validate:"omitempty,max=20,dive"`
	Certificates []CreateCertInput    `json:"certificates" validate:"omitempty,max=20,dive"`
}

type CreateProjectInput struct {
	ID           string   `json:"id,omitempty"`
	Title        string   `json:"title" validate:"required,min=2,max=100"`
	Technologies []string `json:"technologies" validate:"required,min=1,max=10,dive,min=1,max=30"`
	Link         string   `json:"link" validate:"omitempty,max=255"`
	Year         int      `json:"year" validate:"required,min=2000,max=2100"`
}

type CreateCertInput struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name" validate:"required,min=2,max=100"`
	Issuer string `json:"issuer" validate:"required,min=2,max=100"`
	Date   string `json:"date" validate:"required"`
}

// CreateProjectRequest digunakan untuk POST /students/:id/portfolio/projects
type CreateProjectRequest struct {
	Title        string   `json:"title" validate:"required,min=2,max=100"`
	Technologies []string `json:"technologies" validate:"required,min=1,max=10,dive,min=1,max=30"`
	Link         string   `json:"link" validate:"omitempty,max=255"`
	Year         int      `json:"year" validate:"required,min=2000,max=2100"`
}

// TechnologyStat adalah hasil agregasi top 10 teknologi pada portofolio.
type TechnologyStat struct {
	Technology string `json:"technology"`
	Count      int    `json:"count"`
}
