package service

import (
	"context"
	"errors"

	model "api-students/app/model"
	"api-students/app/repository"
	"api-students/helper"

	"github.com/jackc/pgx/v5"
)

type StudentService struct {
	repository    repository.StudentRepository
	permissionSet *helper.PermissionSet
}

func NewStudentService(
	repo repository.StudentRepository,
	permissionSet *helper.PermissionSet,
) *StudentService {
	return &StudentService{
		repository:    repo,
		permissionSet: permissionSet,
	}
}

func (s *StudentService) FindAll(ctx context.Context) ([]model.Student, error) {
	return s.repository.FindAll(ctx)
}

func (s *StudentService) FindByID(
	ctx context.Context,
	id int,
	currentUser model.AuthUser,
) (*model.Student, error) {
	student, err := s.repository.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &NotFoundError{
				Message: "student tidak ditemukan",
			}
		}
		return nil, err
	}

	if !CanAccessStudent(
		currentUser,
		student.OwnerID,
		s.permissionSet,
		"student:read:any",
	) {
		return nil, &ForbiddenError{
			Message: "tidak memiliki akses ke student ini",
		}
	}

	return student, nil
}

func (s *StudentService) Create(
	ctx context.Context,
	req model.CreateStudentRequest,
	ownerID int,
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
		Grade:    int(req.Grade),
		IsActive: req.IsActive,
		OwnerID:  ownerID,
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
	currentUser model.AuthUser,
) (*model.Student, error) {
	current, err := s.repository.FindByID(ctx, id)

	if !CanAccessStudent(currentUser, current.OwnerID, s.permissionSet, "student:update:any") {
		return nil, &ForbiddenError{Message: "tidak memiliki akses untuk mengubah student ini"}
	}

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
	current.Grade = int(req.Grade)
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
	currentUser model.AuthUser,
) (*model.Student, error) {

	current, err := s.repository.FindByID(ctx, id)

	if !CanAccessStudent(currentUser, current.OwnerID, s.permissionSet, "student:update:any") {
		return nil, &ForbiddenError{Message: "tidak memiliki akses untuk mengubah student ini"}
	}
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

type ForbiddenError struct {
	Message string
}

func (e *ForbiddenError) Error() string {
	return e.Message
}
