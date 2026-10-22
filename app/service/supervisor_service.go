package service

import (
	"context"
	"errors"
	"strconv"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/helper"

	"github.com/gofiber/fiber/v2"
)

var (
	errSameSupervisor = errors.New("dosen pembimbing tujuan tidak boleh sama dengan pembimbing asal")
	errMismatchFrom   = errors.New("dosen pembimbing asal tidak sesuai dengan data bimbingan mahasiswa")
)

type SupervisorService struct {
	supervisors repository.SupervisorRepository
	tx          *repository.TxManager
}

func NewSupervisorService(supervisors repository.SupervisorRepository, tx *repository.TxManager) *SupervisorService {
	return &SupervisorService{
		supervisors: supervisors,
		tx:          tx,
	}
}

// GET /api/v1/supervisors
func (s *SupervisorService) List(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	list, err := s.supervisors.FindAll(ctx)
	if err != nil {
		return helper.Internal(err)
	}
	return helper.Success(c, fiber.StatusOK, "daftar dosen pembimbing berhasil diambil", list)
}

// GET /api/v1/supervisors/:id
func (s *SupervisorService) Get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	sup, err := s.supervisors.FindByID(ctx, id)
	if err != nil {
		return translateSupervisorError(err)
	}
	return helper.Success(c, fiber.StatusOK, "dosen pembimbing ditemukan", sup)
}

// POST /api/v1/supervisors/:id/assign
func (s *SupervisorService) Assign(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	supervisorID, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id dosen pembimbing harus berupa angka positif")
	}

	var req model.AssignSupervisorRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	assigned, err := s.AssignSupervisor(ctx, supervisorID, req.StudentID)
	if err != nil {
		return translateSupervisorError(err)
	}

	return helper.Created(c, "berhasil menetapkan dosen pembimbing", assigned,
		"/api/v1/supervisors/"+strconv.Itoa(supervisorID))
}

// AssignSupervisor menjalankan transaksi penetapan pembimbing dengan kunci eksplisit pada baris mahasiswa
// dan atomic conditional increment pada kuota pembimbing.
func (s *SupervisorService) AssignSupervisor(ctx context.Context, supervisorID, studentID int) (model.Supervisor, error) {
	var result model.Supervisor

	err := s.tx.WithTx(ctx, func(tx repository.DBTX) error {
		repo := s.supervisors.WithTx(tx)

		// 1. Kunci baris students untuk mahasiswa ini secara eksklusif (SELECT ... FOR UPDATE).
		// Mencegah dua request serentak menetapkan mahasiswa yang sama ke dua pembimbing berbeda.
		if err := repo.LockStudent(ctx, studentID); err != nil {
			return err
		}

		// 2. Periksa apakah mahasiswa sudah memiliki pembimbing.
		existing, err := repo.FindByStudentID(ctx, studentID)
		if err != nil {
			return err
		}
		if existing != nil {
			return repository.ErrAlreadySupervised
		}

		// 3. Tambah kuota pembimbing secara atomik dan bersyarat (supervised_count < quota).
		// Jika kuota sudah penuh, return ErrSupervisorFull.
		updatedSup, err := repo.IncrementSupervised(ctx, supervisorID)
		if err != nil {
			return err
		}

		// 4. Catat penetapan pembimbing ke tabel supervisions.
		if err := repo.InsertSupervision(ctx, studentID, supervisorID); err != nil {
			return err
		}

		result = updatedSup
		return nil
	})

	return result, err
}

// POST /api/v1/supervisions/switch
func (s *SupervisorService) Switch(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	var req model.SwitchSupervisorRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	if req.FromSupervisorID != nil && *req.FromSupervisorID == req.ToSupervisorID {
		return helper.Validation(map[string]string{
			"to_supervisor_id": "tidak boleh sama dengan from_supervisor_id",
		})
	}

	switched, err := s.SwitchSupervisor(ctx, req.StudentID, req.ToSupervisorID, req.FromSupervisorID)
	if err != nil {
		return translateSupervisorError(err)
	}

	return helper.Success(c, fiber.StatusOK, "berhasil memindahkan dosen pembimbing", switched)
}

// SwitchSupervisor memindahkan mahasiswa ke pembimbing baru secara atomik.
// Jika pembimbing baru penuh, transaksi di-rollback sehingga mahasiswa tetap tercatat pada pembimbing lama.
// Urutan penguncian pembimbing selalu menaik berdasarkan ID untuk mencegah deadlock.
func (s *SupervisorService) SwitchSupervisor(ctx context.Context, studentID, toSupervisorID int, fromSupervisorID *int) (model.Supervisor, error) {
	var result model.Supervisor

	err := s.tx.WithTx(ctx, func(tx repository.DBTX) error {
		repo := s.supervisors.WithTx(tx)

		// 1. Kunci baris mahasiswa untuk menjamin operasi eksklusif.
		if err := repo.LockStudent(ctx, studentID); err != nil {
			return err
		}

		// 2. Dapatkan pembimbing saat ini.
		current, err := repo.FindByStudentID(ctx, studentID)
		if err != nil {
			return err
		}
		if current == nil {
			return repository.ErrNotSupervised
		}

		if fromSupervisorID != nil && current.ID != *fromSupervisorID {
			return errMismatchFrom
		}

		if current.ID == toSupervisorID {
			return errSameSupervisor
		}

		fromID := current.ID
		toID := toSupervisorID

		// 3. Kunci KEDUA baris pembimbing SELALU dalam urutan ID menaik untuk menghindari deadlock.
		firstID, secondID := fromID, toID
		if firstID > secondID {
			firstID, secondID = secondID, firstID
		}
		if err := repo.LockSupervisors(ctx, firstID, secondID); err != nil {
			return err
		}

		// 4. Tambah pembimbing baru secara atomik bersyarat (supervised_count < quota).
		// Jika penuh, IncrementSupervised akan gagal dan membatalkan seluruh transaksi,
		// sehingga mahasiswa tetap aman bersama pembimbing lama.
		newSup, err := repo.IncrementSupervised(ctx, toID)
		if err != nil {
			return err
		}

		// 5. Kurangi mahasiswa dari pembimbing lama.
		if _, err := repo.DecrementSupervised(ctx, fromID); err != nil {
			return err
		}

		// 6. Perbarui baris pada tabel supervisions.
		if err := repo.UpdateSupervision(ctx, studentID, toID); err != nil {
			return err
		}

		result = newSup
		return nil
	})

	return result, err
}

// PATCH /api/v1/supervisors/:id
func (s *SupervisorService) UpdateQuota(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id dosen pembimbing harus berupa angka positif")
	}

	var req model.UpdateSupervisorQuotaRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	updated, err := s.UpdateSupervisorQuota(ctx, id, req.Quota, req.Version)
	if err != nil {
		return translateSupervisorError(err)
	}

	return helper.Success(c, fiber.StatusOK, "kuota dosen pembimbing berhasil diubah", updated)
}

// UpdateSupervisorQuota mengubah kuota dengan optimistic locking.
func (s *SupervisorService) UpdateSupervisorQuota(ctx context.Context, supervisorID, quota, version int) (model.Supervisor, error) {
	return s.supervisors.UpdateQuota(ctx, supervisorID, quota, version)
}

// translateSupervisorError memetakan error repository/service ke HTTP AppError standar.
func translateSupervisorError(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return helper.NotFound("mahasiswa tidak ditemukan")
	case errors.Is(err, repository.ErrSupervisorNotFound):
		return helper.NotFound("dosen pembimbing tidak ditemukan")
	case errors.Is(err, repository.ErrSupervisorFull):
		return helper.NewError(fiber.StatusConflict, "SUPERVISOR_FULL", "kuota dosen pembimbing sudah penuh")
	case errors.Is(err, repository.ErrAlreadySupervised):
		return helper.NewError(fiber.StatusConflict, "ALREADY_SUPERVISED", "mahasiswa sudah memiliki dosen pembimbing")
	case errors.Is(err, repository.ErrNotSupervised):
		return helper.NewError(fiber.StatusConflict, "NOT_SUPERVISED", "mahasiswa belum memiliki dosen pembimbing")
	case errors.Is(err, errMismatchFrom):
		return helper.NewError(fiber.StatusConflict, "SUPERVISOR_MISMATCH", "dosen pembimbing asal tidak sesuai dengan data bimbingan mahasiswa")
	case errors.Is(err, errSameSupervisor):
		return helper.NewError(fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", "dosen pembimbing tujuan tidak boleh sama dengan pembimbing asal")
	case errors.Is(err, repository.ErrSupervisorVersionConflict):
		return helper.NewError(fiber.StatusConflict, "VERSION_CONFLICT", "data dosen pembimbing sudah diubah pihak lain, muat ulang lalu coba lagi")
	case errors.Is(err, repository.ErrQuotaBelowSupervised):
		return helper.NewError(fiber.StatusConflict, "QUOTA_BELOW_SUPERVISED", "kuota tidak boleh lebih kecil dari jumlah mahasiswa yang sedang dibimbing")
	default:
		return helper.Internal(err)
	}
}
