package repository

import (
	"context"
	"errors"
	"fmt"

	"api-students/app/model"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSupervisorNotFound        = errors.New("dosen pembimbing tidak ditemukan")
	ErrSupervisorFull            = errors.New("kuota dosen pembimbing sudah penuh")
	ErrAlreadySupervised         = errors.New("mahasiswa sudah memiliki dosen pembimbing")
	ErrNotSupervised             = errors.New("mahasiswa belum memiliki dosen pembimbing")
	ErrSupervisorVersionConflict = errors.New("data dosen pembimbing sudah diubah pihak lain")
	ErrQuotaBelowSupervised      = errors.New("kuota tidak boleh lebih kecil dari jumlah mahasiswa yang dibimbing")
)

type SupervisorRepository interface {
	WithTx(tx DBTX) SupervisorRepository
	FindAll(ctx context.Context) ([]model.Supervisor, error)
	FindByID(ctx context.Context, id int) (model.Supervisor, error)
	FindByStudentID(ctx context.Context, studentID int) (*model.Supervisor, error)
	LockStudent(ctx context.Context, studentID int) error
	LockSupervisors(ctx context.Context, ids ...int) error
	InsertSupervision(ctx context.Context, studentID, supervisorID int) error
	UpdateSupervision(ctx context.Context, studentID, newSupervisorID int) error
	DeleteSupervision(ctx context.Context, studentID int) error
	IncrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error)
	DecrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error)
	UpdateQuota(ctx context.Context, id, quota, version int) (model.Supervisor, error)
}

const supervisorColumns = `id, name, quota, supervised_count, version, created_at`

type supervisorPostgresRepository struct {
	db DBTX
}

func NewSupervisorRepository(db DBTX) SupervisorRepository {
	return &supervisorPostgresRepository{db: db}
}

func (r *supervisorPostgresRepository) WithTx(tx DBTX) SupervisorRepository {
	return &supervisorPostgresRepository{db: tx}
}

func scanSupervisor(row pgx.Row) (model.Supervisor, error) {
	var s model.Supervisor
	err := row.Scan(&s.ID, &s.Name, &s.Quota, &s.SupervisedCount, &s.Version, &s.CreatedAt)
	return s, err
}

func (r *supervisorPostgresRepository) collect(rows pgx.Rows, err error) ([]model.Supervisor, error) {
	if err != nil {
		return nil, fmt.Errorf("mengambil daftar dosen pembimbing: %w", err)
	}
	defer rows.Close()

	result := []model.Supervisor{}
	for rows.Next() {
		s, err := scanSupervisor(rows)
		if err != nil {
			return nil, fmt.Errorf("membaca row dosen pembimbing: %w", err)
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *supervisorPostgresRepository) FindAll(ctx context.Context) ([]model.Supervisor, error) {
	return r.collect(r.db.Query(ctx, "SELECT "+supervisorColumns+" FROM supervisors ORDER BY id"))
}

func (r *supervisorPostgresRepository) FindByID(ctx context.Context, id int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx, "SELECT "+supervisorColumns+" FROM supervisors WHERE id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Supervisor{}, ErrSupervisorNotFound
		}
		return model.Supervisor{}, fmt.Errorf("mengambil dosen pembimbing: %w", err)
	}
	return s, nil
}

func (r *supervisorPostgresRepository) FindByStudentID(ctx context.Context, studentID int) (*model.Supervisor, error) {
	var s model.Supervisor
	err := r.db.QueryRow(ctx,
		`SELECT s.id, s.name, s.quota, s.supervised_count, s.version, s.created_at
         FROM supervisors s
         JOIN supervisions sp ON sp.supervisor_id = s.id
         WHERE sp.student_id = $1`, studentID).Scan(
		&s.ID, &s.Name, &s.Quota, &s.SupervisedCount, &s.Version, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("mengambil dosen pembimbing mahasiswa: %w", err)
	}
	return &s, nil
}

// LockStudent mengunci baris students milik mahasiswa secara eksplisit (SELECT ... FOR UPDATE)
// untuk mencegah race condition penetapan ganda ke dua pembimbing sekaligus.
func (r *supervisorPostgresRepository) LockStudent(ctx context.Context, studentID int) error {
	var id int
	err := r.db.QueryRow(ctx, "SELECT id FROM students WHERE id = $1 FOR UPDATE", studentID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("mengunci data mahasiswa: %w", err)
	}
	return nil
}

// LockSupervisors mengunci baris pembimbing SELALU dalam urutan id menaik (ORDER BY id)
// untuk mencegah deadlock saat terjadi switch silang.
func (r *supervisorPostgresRepository) LockSupervisors(ctx context.Context, ids ...int) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := r.db.Query(ctx,
		"SELECT id FROM supervisors WHERE id = ANY($1) ORDER BY id FOR UPDATE", ids)
	if err != nil {
		return fmt.Errorf("mengunci dosen pembimbing: %w", err)
	}
	defer rows.Close()

	found := 0
	for rows.Next() {
		found++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("mengunci dosen pembimbing: %w", err)
	}
	if found != len(ids) {
		return ErrSupervisorNotFound
	}
	return nil
}

func (r *supervisorPostgresRepository) InsertSupervision(ctx context.Context, studentID, supervisorID int) error {
	_, err := r.db.Exec(ctx,
		"INSERT INTO supervisions (student_id, supervisor_id) VALUES ($1, $2)", studentID, supervisorID)
	if err != nil {
		if pgErrorCode(err) == codeUniqueViolation {
			return ErrAlreadySupervised
		}
		return fmt.Errorf("mencatat pembimbingan: %w", err)
	}
	return nil
}

func (r *supervisorPostgresRepository) UpdateSupervision(ctx context.Context, studentID, newSupervisorID int) error {
	tag, err := r.db.Exec(ctx,
		"UPDATE supervisions SET supervisor_id = $1, created_at = NOW() WHERE student_id = $2",
		newSupervisorID, studentID)
	if err != nil {
		return fmt.Errorf("mengubah dosen pembimbing: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotSupervised
	}
	return nil
}

func (r *supervisorPostgresRepository) DeleteSupervision(ctx context.Context, studentID int) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM supervisions WHERE student_id = $1", studentID)
	if err != nil {
		return fmt.Errorf("menghapus penetapan pembimbing: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotSupervised
	}
	return nil
}

// IncrementSupervised menambah supervised_count secara kondisional (supervised_count < quota).
func (r *supervisorPostgresRepository) IncrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors SET supervised_count = supervised_count + 1
         WHERE id = $1 AND supervised_count < quota
         RETURNING `+supervisorColumns, supervisorID))
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Supervisor{}, fmt.Errorf("menambah mahasiswa bimbingan: %w", err)
	}

	if _, findErr := r.FindByID(ctx, supervisorID); findErr != nil {
		return model.Supervisor{}, findErr
	}
	return model.Supervisor{}, ErrSupervisorFull
}

func (r *supervisorPostgresRepository) DecrementSupervised(ctx context.Context, supervisorID int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors SET supervised_count = supervised_count - 1
         WHERE id = $1 AND supervised_count > 0
         RETURNING `+supervisorColumns, supervisorID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Supervisor{}, ErrSupervisorNotFound
		}
		return model.Supervisor{}, fmt.Errorf("mengurangi mahasiswa bimbingan: %w", err)
	}
	return s, nil
}

// UpdateQuota menggunakan optimistic locking dengan version = version + 1 dan pengecekan quota >= supervised_count.
func (r *supervisorPostgresRepository) UpdateQuota(ctx context.Context, id, quota, version int) (model.Supervisor, error) {
	s, err := scanSupervisor(r.db.QueryRow(ctx,
		`UPDATE supervisors SET quota = $1, version = version + 1
         WHERE id = $2 AND version = $3 AND $1 >= supervised_count
         RETURNING `+supervisorColumns, quota, id, version))
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Supervisor{}, fmt.Errorf("mengubah kuota pembimbing: %w", err)
	}

	current, findErr := r.FindByID(ctx, id)
	if findErr != nil {
		return model.Supervisor{}, findErr
	}
	if current.Version != version {
		return model.Supervisor{}, ErrSupervisorVersionConflict
	}
	return model.Supervisor{}, ErrQuotaBelowSupervised
}
