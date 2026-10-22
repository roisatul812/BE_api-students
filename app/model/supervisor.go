package model

import "time"

type Supervisor struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Quota           int       `json:"quota"`
	SupervisedCount int       `json:"supervised_count"`
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
}

type Supervision struct {
	StudentID    int       `json:"student_id"`
	SupervisorID int       `json:"supervisor_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// AssignSupervisorRequest dipakai untuk penetapan pembimbing bagi mahasiswa.
type AssignSupervisorRequest struct {
	StudentID int `json:"student_id" validate:"required,min=1"`
}

// SwitchSupervisorRequest dipakai untuk memindahkan mahasiswa ke pembimbing lain secara atomik.
type SwitchSupervisorRequest struct {
	StudentID        int  `json:"student_id"          validate:"required,min=1"`
	FromSupervisorID *int `json:"from_supervisor_id"  validate:"omitempty,min=1"`
	ToSupervisorID   int  `json:"to_supervisor_id"    validate:"required,min=1"`
}

// UpdateSupervisorQuotaRequest membawa version yang terakhir dilihat client untuk optimistic locking.
type UpdateSupervisorQuotaRequest struct {
	Quota   int `json:"quota"   validate:"required,min=1,max=500"`
	Version int `json:"version" validate:"required,min=1"`
}
