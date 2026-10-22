package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	model "api-students/app/model"
	"api-students/app/repository"
	"api-students/helper"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (s *StudentService) FindByCursor(
	ctx context.Context,
	rawCursor string,
	limit int,
) (*model.StudentCursorResponse, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var cursorParam *model.CursorParam
	if strings.TrimSpace(rawCursor) != "" {
		c, err := helper.DecodeCursor(rawCursor)
		if err != nil {
			return nil, err
		}
		cursorParam = &model.CursorParam{
			CreatedAt: c.CreatedAt,
			ID:        c.ID,
		}
	}

	students, hasMore, nextCursor, err := s.repository.FindByCursor(ctx, cursorParam, limit)
	if err != nil {
		return nil, TranslateError(err, "student")
	}

	var nextCursorPtr *string
	if hasMore && nextCursor != "" {
		nextCursorPtr = &nextCursor
	}

	return &model.StudentCursorResponse{
		Data: students,
		Meta: model.CursorMeta{
			Limit:      limit,
			NextCursor: nextCursorPtr,
			HasMore:    hasMore,
		},
	}, nil
}

func (s *StudentService) FindByID(
	ctx context.Context,
	id int,
	currentUser model.AuthUser,
) (*model.Student, error) {

	student, err := s.repository.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helper.NotFound("student tidak ditemukan")
		}

		return nil, TranslateError(err, "student")
	}

	if !CanAccessStudent(
		currentUser,
		student.OwnerID,
		s.permissionSet,
		"student:read:any",
	) {
		return nil, helper.Forbidden("tidak memiliki akses ke student ini")
	}

	return student, nil
}

func (s *StudentService) Create(
	ctx context.Context,
	req model.CreateStudentRequest,
	ownerID int,
) (*model.Student, error) {

	errs := helper.ValidateStruct(req)
	if len(errs) > 0 {
		return nil, helper.Validation(errs)
	}

	students, err := s.repository.FindAll(ctx)
	if err != nil {
		return nil, TranslateError(err, "student")
	}

	for _, student := range students {
		if student.NIM == req.NIM {
			return nil, helper.Conflict("NIM sudah digunakan")
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
		return nil, TranslateError(err, "student")
	}

	return student, nil
}

func (s *StudentService) Replace(
	ctx context.Context,
	id int,
	req model.ReplaceStudentRequest,
	currentUser model.AuthUser,
) (*model.Student, error) {

	// Cari student terlebih dahulu.
	current, err := s.repository.FindByID(ctx, id)

	// WAJIB cek error sebelum mengakses current.OwnerID.
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helper.NotFound("student tidak ditemukan")
		}

		return nil, TranslateError(err, "student")
	}

	// Setelah dipastikan current tidak nil, baru cek permission.
	if !CanAccessStudent(
		currentUser,
		current.OwnerID,
		s.permissionSet,
		"student:update:any",
	) {
		return nil, helper.Forbidden("tidak memiliki akses untuk mengubah student ini")
	}

	// Validasi request PUT.
	errs := helper.ValidateStruct(req)
	if len(errs) > 0 {
		return nil, helper.Validation(errs)
	}

	// Cek apakah NIM sudah digunakan student lain.
	students, err := s.repository.FindAll(ctx)
	if err != nil {
		return nil, TranslateError(err, "student")
	}

	for _, student := range students {
		if student.NIM == req.NIM && student.ID != id {
			return nil, helper.Conflict("NIM sudah digunakan")
		}
	}

	// Replace seluruh field yang diperbolehkan.
	current.NIM = req.NIM
	current.Name = req.Name
	current.Grade = int(req.Grade)
	current.IsActive = req.IsActive

	// Simpan perubahan.
	if err := s.repository.Update(ctx, current); err != nil {
		return nil, TranslateError(err, "student")
	}

	return current, nil
}

func (s *StudentService) Patch(
	ctx context.Context,
	id int,
	req model.PatchStudentRequest,
	currentUser model.AuthUser,
) (*model.Student, error) {

	// Cari student terlebih dahulu.
	current, err := s.repository.FindByID(ctx, id)

	// WAJIB cek error sebelum mengakses current.OwnerID.
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, helper.NotFound("student tidak ditemukan")
		}

		return nil, TranslateError(err, "student")
	}

	// Setelah current dipastikan valid, cek permission.
	if !CanAccessStudent(
		currentUser,
		current.OwnerID,
		s.permissionSet,
		"student:update:any",
	) {
		return nil, helper.Forbidden("tidak memiliki akses untuk mengubah student ini")
	}

	// PATCH minimal harus memiliki satu field.
	if IsEmptyPatch(req) {
		return nil, helper.Validation(map[string]string{
			"body": "minimal satu field harus dikirim",
		})
	}

	// Validasi deklaratif untuk field pointer pada request PATCH.
	if errs := helper.ValidateStruct(req); len(errs) > 0 {
		return nil, helper.Validation(errs)
	}

	// Kalau NIM diubah, pastikan tidak bentrok dengan student lain.
	if req.NIM != nil {
		students, err := s.repository.FindAll(ctx)
		if err != nil {
			return nil, TranslateError(err, "student")
		}

		for _, student := range students {
			if student.NIM == *req.NIM && student.ID != id {
				return nil, helper.Conflict("NIM sudah digunakan")
			}
		}
	}

	// Terapkan perubahan PATCH (validasi nilai field telah dilakukan secara deklaratif di atas).
	updated := ApplyPatch(*current, req)

	// Simpan perubahan.
	if err := s.repository.Update(ctx, &updated); err != nil {
		return nil, TranslateError(err, "student")
	}

	return &updated, nil
}

func (s *StudentService) Delete(
	ctx context.Context,
	id *int,
	currentUser model.AuthUser,
) error {
	student, err := s.repository.FindByID(ctx, *id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return helper.NotFound("student tidak ditemukan")
		}
		return TranslateError(err, "student")
	}

	if !CanAccessStudent(
		currentUser,
		student.OwnerID,
		s.permissionSet,
		"student:delete:any",
	) {
		return helper.Forbidden("tidak memiliki akses untuk menghapus student ini")
	}

	if err := s.repository.Delete(ctx, *id); err != nil {
		return TranslateError(err, "student")
	}

	return nil
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

// TranslateError mengonversi error database menjadi *helper.AppError
// sesuai prinsip fail-closed (Modul 7 Langkah 4).
// Planted error Modul 7 memiliki default: return nil yang menelan unknown database error.
// Perbaikan ini memastikan default fail-closed mengembalikan helper.Internal(err).
func TranslateError(err error, entity string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return helper.NotFound(fmt.Sprintf("%s tidak ditemukan", entity))
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return helper.Conflict(fmt.Sprintf("%s dengan data tersebut sudah ada", entity))
		case "23503": // foreign_key_violation
			return helper.BadRequest("referensi data tidak valid")
		}
	}

	// PERBAIKAN BUG #5: Default harus fail-closed!
	return helper.Internal(err)
}

