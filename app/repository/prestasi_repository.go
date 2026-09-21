package repository

import (
	"context"

	"api-students/app/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PrestasiRepository interface {
	FindByNIM(ctx context.Context, nim string) ([]model.Prestasi, error)
}

type prestasiPostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPrestasiRepository(pool *pgxpool.Pool) PrestasiRepository {
	return &prestasiPostgresRepository{
		pool: pool,
	}
}

func (r *prestasiPostgresRepository) FindByNIM(
	ctx context.Context,
	nim string,
) ([]model.Prestasi, error) {
	rows, err := r.pool.Query(
		ctx,
		`
		SELECT
			p.id,
			p.nama_prestasi,
			p.student_id,
			p.juara
		FROM prestasi p
		INNER JOIN students s ON s.id = p.student_id
		WHERE s.nim = $1
		ORDER BY p.id
		`,
		nim,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prestasiList []model.Prestasi

	for rows.Next() {
		var prestasi model.Prestasi

		err := rows.Scan(
			&prestasi.ID,
			&prestasi.NamaPrestasi,
			&prestasi.StudentID,
			&prestasi.Juara,
		)
		if err != nil {
			return nil, err
		}

		prestasiList = append(prestasiList, prestasi)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return prestasiList, nil
}
