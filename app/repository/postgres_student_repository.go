package repository

import (
	"context"
	"time"

	model "api-students/app/model"
	"api-students/helper"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStudentRepository struct {
	DB *pgxpool.Pool
}

func NewPostgresStudentRepository(db *pgxpool.Pool) *PostgresStudentRepository {
	return &PostgresStudentRepository{
		DB: db,
	}
}

func (r *PostgresStudentRepository) FindAll(ctx context.Context) ([]model.Student, error) {
	query := `
		SELECT id, nim, name, grade, is_active, owner_id, created_at
		FROM students
		ORDER BY id
	`

	rows, err := r.DB.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var students []model.Student

	for rows.Next() {
		var student model.Student

		err := rows.Scan(
			&student.ID,
			&student.NIM,
			&student.Name,
			&student.Grade,
			&student.IsActive,
			&student.OwnerID,
			&student.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		students = append(students, student)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return students, nil
}

// FindByCursor mengimplementasikan keyset/cursor pagination (Modul 7 Langkah 7).
// PERBAIKAN BUG #8:
// Menggunakan ORDER BY created_at DESC, id DESC secara stabil dan konsisten dengan keyset (created_at, id) < ($1, $2).
func (r *PostgresStudentRepository) FindByCursor(ctx context.Context, cursor *model.CursorParam, limit int) ([]model.Student, bool, string, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	fetchLimit := limit + 1
	var rows pgx.Rows
	var err error

	if cursor == nil {
		query := `
			SELECT id, nim, name, grade, is_active, owner_id, created_at
			FROM students
			ORDER BY created_at DESC, id DESC
			LIMIT $1
		`
		rows, err = r.DB.Query(ctx, query, fetchLimit)
	} else {
		query := `
			SELECT id, nim, name, grade, is_active, owner_id, created_at
			FROM students
			WHERE (created_at, id) < ($1, $2)
			ORDER BY created_at DESC, id DESC
			LIMIT $3
		`
		rows, err = r.DB.Query(ctx, query, cursor.CreatedAt, cursor.ID, fetchLimit)
	}
	if err != nil {
		return nil, false, "", err
	}
	defer rows.Close()

	var students []model.Student
	for rows.Next() {
		var student model.Student
		if err := rows.Scan(
			&student.ID,
			&student.NIM,
			&student.Name,
			&student.Grade,
			&student.IsActive,
			&student.OwnerID,
			&student.CreatedAt,
		); err != nil {
			return nil, false, "", err
		}
		students = append(students, student)
	}

	if err := rows.Err(); err != nil {
		return nil, false, "", err
	}

	hasMore := len(students) > limit
	var nextCursor string
	if hasMore {
		students = students[:limit]
		last := students[limit-1]
		nextCursor = helper.EncodeCursor(last.CreatedAt, last.ID)
	}

	if students == nil {
		students = []model.Student{}
	}

	return students, hasMore, nextCursor, nil
}


func (r *PostgresStudentRepository) FindByID(
	ctx context.Context,
	id int,
) (*model.Student, error) {
	query := `
		SELECT id, nim, name, grade, is_active, owner_id,created_at
		FROM students
		WHERE id = $1
	`

	var student model.Student

	err := r.DB.QueryRow(ctx, query, id).Scan(
		&student.ID,
		&student.NIM,
		&student.Name,
		&student.Grade,
		&student.IsActive,
		&student.OwnerID,
		&student.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &student, nil
}

func (r *PostgresStudentRepository) Create(
	ctx context.Context,
	student *model.Student,
) error {
	query := `
		INSERT INTO students (
			nim,
			name,
			grade,
			is_active,
			owner_id,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	if student.CreatedAt.IsZero() {
		student.CreatedAt = time.Now()
	}

	return r.DB.QueryRow(
		ctx,
		query,
		student.NIM,
		student.Name,
		student.Grade,
		student.IsActive,
		student.OwnerID,
		student.CreatedAt,
	).Scan(&student.ID)
}

func (r *PostgresStudentRepository) Update(
	ctx context.Context,
	student *model.Student,
) error {
	query := `
		UPDATE students
		SET
			nim = $1,
			name = $2,
			grade = $3,
			is_active = $4
		WHERE id = $5
	`

	_, err := r.DB.Exec(
		ctx,
		query,
		student.NIM,
		student.Name,
		student.Grade,
		student.IsActive,
		student.ID,
	)

	return err
}

func (r *PostgresStudentRepository) Delete(
	ctx context.Context,
	id int,
) error {
	query := `
		DELETE FROM students
		WHERE id = $1
	`

	_, err := r.DB.Exec(ctx, query, id)

	return err
}
