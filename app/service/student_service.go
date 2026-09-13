package service

import (
	"context"
	"errors"

	model "api-students/app/model"
	"api-students/app/repository"

	"github.com/jackc/pgx/v5"
)

type StudentService struct {
	repository repository.StudentRepository
}

func NewStudentService(repo repository.StudentRepository) *StudentService {
	return &StudentService{
		repository: repo,
	}
}

func (s *StudentService) FindAll(ctx context.Context) ([]model.Student, error) {
	return s.repository.FindAll(ctx)
}

func (s *StudentService) FindByID(ctx context.Context, id int) (*model.Student, error) {
	student, err := s.repository.FindByID(ctx, id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{
				Message: "student tidak ditemukan",
			}
		}

		return nil, err
	}

	return student, nil
}

func (s *StudentService) Create(
	ctx context.Context,
	req model.CreateStudentRequest,
) (*model.Student, error) {

	errs := ValidateCreate(req)

	if len(errs) > 0 {
		return nil, &ValidationError{
			Errors: errs,
		}
	}

	students, err := s.repository.FindAll(ctx)

	if err != nil {
		return nil, err
	}

	for _, student := range students {
		if student.NIM == req.NIM {
			return nil, &ConflictError{
				Message: "NIM sudah digunakan",
			}
		}
	}

	student := &model.Student{
		NIM:      req.NIM,
		Name:     req.Name,
		Grade:    req.Grade,
		IsActive: req.IsActive,
	}

	if err := s.repository.Create(ctx, student); err != nil {
		return nil, err
	}

	return student, nil
}

func (s *StudentService) Replace(
	ctx context.Context,
	id int,
	req model.ReplaceStudentRequest,
) (*model.Student, error) {

	current, err := s.repository.FindByID(ctx, id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{
				Message: "student tidak ditemukan",
			}
		}

		return nil, err
	}

	errs := ValidateReplace(req)

	if len(errs) > 0 {
		return nil, &ValidationError{
			Errors: errs,
		}
	}

	students, err := s.repository.FindAll(ctx)

	if err != nil {
		return nil, err
	}

	for _, student := range students {
		if student.NIM == req.NIM && student.ID != id {
			return nil, &ConflictError{
				Message: "NIM sudah digunakan",
			}
		}
	}

	current.NIM = req.NIM
	current.Name = req.Name
	current.Grade = req.Grade
	current.IsActive = req.IsActive

	if err := s.repository.Update(ctx, current); err != nil {
		return nil, err
	}

	return current, nil
}

func (s *StudentService) Patch(
	ctx context.Context,
	id int,
	req model.PatchStudentRequest,
) (*model.Student, error) {

	current, err := s.repository.FindByID(ctx, id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{
				Message: "student tidak ditemukan",
			}
		}

		return nil, err
	}

	if IsEmptyPatch(req) {
		return nil, &ValidationError{
			Errors: map[string]string{
				"body": "minimal satu field harus dikirim",
			},
		}
	}

	if req.NIM != nil {
		students, err := s.repository.FindAll(ctx)

		if err != nil {
			return nil, err
		}

		for _, student := range students {
			if student.NIM == *req.NIM && student.ID != id {
				return nil, &ConflictError{
					Message: "NIM sudah digunakan",
				}
			}
		}
	}

	updated, errs := ApplyPatch(*current, req)

	if len(errs) > 0 {
		return nil, &ValidationError{
			Errors: errs,
		}
	}

	if err := s.repository.Update(ctx, &updated); err != nil {
		return nil, err
	}

	return &updated, nil
}

func (s *StudentService) Delete(ctx context.Context, id int) error {
	_, err := s.repository.FindByID(ctx, id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &NotFoundError{
				Message: "student tidak ditemukan",
			}
		}

		return err
	}

	return s.repository.Delete(ctx, id)
}

type ValidationError struct {
	Errors map[string]string
}

func (e *ValidationError) Error() string {
	return "validasi gagal"
}

type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string {
	return e.Message
}

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}
