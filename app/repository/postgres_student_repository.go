package repository

import (
	"context"
	"time"

	model "api-students/app/model"

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
		SELECT id, nim, name, grade, is_active, created_at
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

func (r *PostgresStudentRepository) FindByID(
	ctx context.Context,
	id int,
) (*model.Student, error) {
	query := `
		SELECT id, nim, name, grade, is_active, created_at
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
			created_at
		)
		VALUES ($1, $2, $3, $4, $5)
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
