package repository_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"api-students/app/repository"
	"api-students/app/service"

	"github.com/jackc/pgx/v5/pgxpool"
)

// resetLoadtestData membersihkan data pendaftaran mhs001-mhs060 dan mereset enrolled_count
func resetLoadtestData(t *testing.T, db *pgxpool.Pool) {
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		DELETE FROM enrollments
		WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'mhs%');
		UPDATE courses c
		SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = c.id);
	`)
	if err != nil {
		t.Fatalf("gagal reset loadtest data: %v", err)
	}
}

// TestConcurrency_CrowdEnrollment_IF399 menguji 60 mahasiswa serentak berebut IF399 (kuota 5).
// Harus menghasilkan tepat 5 berhasil, 55 COURSE_FULL, dan enrolled_count == 5.
// Memverifikasi perbaikan Planted Error 6 (strict inequality < quota).
func TestConcurrency_CrowdEnrollment_IF399(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	resetLoadtestData(t, db)

	// Dapatkan ID kelas IF399
	var courseID, quota int
	err := db.QueryRow(ctx, "SELECT id, quota FROM courses WHERE code = 'IF399'").Scan(&courseID, &quota)
	if err != nil {
		t.Fatalf("gagal mencari kelas IF399: %v", err)
	}
	if quota != 5 {
		t.Fatalf("kuota IF399 diharapkan 5, didapat %d", quota)
	}

	// Dapatkan 60 ID user mahasiswa mhs001-mhs060
	rows, err := db.Query(ctx, "SELECT id FROM users WHERE username LIKE 'mhs%' ORDER BY username LIMIT 60")
	if err != nil {
		t.Fatalf("gagal mengambil data user loadtest: %v", err)
	}
	defer rows.Close()

	var userIDs []int
	for rows.Next() {
		var uid int
		if err := rows.Scan(&uid); err != nil {
			t.Fatalf("scan user id: %v", err)
		}
		userIDs = append(userIDs, uid)
	}
	if len(userIDs) < 60 {
		t.Fatalf("jumlah user loadtest kurang dari 60: %d", len(userIDs))
	}

	courseRepo := repository.NewCourseRepository(db)
	txManager := repository.NewTxManager(db)

	n := 60
	startGate := make(chan struct{})
	var wg sync.WaitGroup
	var successCount int64
	var fullCount int64
	var otherErrorCount int64

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			uid := userIDs[idx]

			// Tunggu sinyal start serentak
			<-startGate

			enrollErr := txManager.WithTx(ctx, func(tx repository.DBTX) error {
				repo := courseRepo.WithTx(tx)

				if err := repo.LockStudent(ctx, uid); err != nil {
					return err
				}

				course, err := repo.FindByID(ctx, courseID)
				if err != nil {
					return err
				}

				taken, err := repo.SumCredits(ctx, uid)
				if err != nil {
					return err
				}
				if !service.WithinCreditLimit(taken, course.Credits) {
					return errors.New("credit limit exceeded")
				}

				if _, err := repo.IncrementEnrolled(ctx, courseID); err != nil {
					return err
				}

				return repo.InsertEnrollment(ctx, courseID, uid)
			})

			if enrollErr == nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(enrollErr, repository.ErrCourseFull) {
				atomic.AddInt64(&fullCount, 1)
			} else {
				atomic.AddInt64(&otherErrorCount, 1)
			}
		}(i)
	}

	// Buka gerbang start serentak
	close(startGate)
	wg.Wait()

	t.Logf("Hasil Uji Beban IF399 (kuota 5, 60 peminat serentak):")
	t.Logf("  Berhasil mendaftar (201): %d", successCount)
	t.Logf("  Ditolak kuota penuh (409 COURSE_FULL): %d", fullCount)
	t.Logf("  Error lainnya: %d", otherErrorCount)

	if successCount != 5 {
		t.Errorf("Ekspektasi tepat 5 pendaftaran berhasil, aktual: %d", successCount)
	}
	if fullCount != 55 {
		t.Errorf("Ekspektasi tepat 55 pendaftaran ditolak COURSE_FULL, aktual: %d", fullCount)
	}
	if otherErrorCount != 0 {
		t.Errorf("Ekspektasi 0 error lainnya, aktual: %d", otherErrorCount)
	}

	// Verifikasi konsistensi di database
	var dbEnrolledCount int
	var rowCount int
	err = db.QueryRow(ctx, "SELECT enrolled_count FROM courses WHERE id = $1", courseID).Scan(&dbEnrolledCount)
	if err != nil {
		t.Fatalf("gagal membaca enrolled_count: %v", err)
	}
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM enrollments WHERE course_id = $1", courseID).Scan(&rowCount)
	if err != nil {
		t.Fatalf("gagal menghitung row enrollments: %v", err)
	}

	t.Logf("Verifikasi Database: enrolled_count=%d, row count=%d", dbEnrolledCount, rowCount)
	if dbEnrolledCount != 5 || rowCount != 5 {
		t.Errorf("Inkonsistensi data: enrolled_count=%d, row count=%d (harus sama-sama 5)", dbEnrolledCount, rowCount)
	}
}

// TestConcurrency_CreditLimit_24SKS menguji 10 mahasiswa masing-masing mendaftar ke 12 kelas 3 SKS secara serentak.
// Total 36 SKS attempted. Batas maksimal 24 SKS.
// Memverifikasi perbaikan Planted Error 5 (SELECT ... FOR UPDATE pada LockStudent).
func TestConcurrency_CreditLimit_24SKS(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	resetLoadtestData(t, db)

	// Ambil 12 kelas 3 SKS berkuota besar
	rows, err := db.Query(ctx, "SELECT id, credits FROM courses WHERE code != 'IF399' ORDER BY id LIMIT 12")
	if err != nil {
		t.Fatalf("gagal mengambil data kelas: %v", err)
	}
	defer rows.Close()

	type courseInfo struct {
		id      int
		credits int
	}
	var courses []courseInfo
	for rows.Next() {
		var c courseInfo
		if err := rows.Scan(&c.id, &c.credits); err != nil {
			t.Fatalf("scan course: %v", err)
		}
		courses = append(courses, c)
	}
	if len(courses) < 12 {
		t.Fatalf("kurang dari 12 kelas tersedia: %d", len(courses))
	}

	// Ambil 10 user mahasiswa
	userRows, err := db.Query(ctx, "SELECT id FROM users WHERE username LIKE 'mhs%' ORDER BY username LIMIT 10")
	if err != nil {
		t.Fatalf("gagal mengambil data user: %v", err)
	}
	defer userRows.Close()

	var userIDs []int
	for userRows.Next() {
		var uid int
		if err := userRows.Scan(&uid); err != nil {
			t.Fatalf("scan user id: %v", err)
		}
		userIDs = append(userIDs, uid)
	}

	courseRepo := repository.NewCourseRepository(db)
	txManager := repository.NewTxManager(db)
	errCreditLimit := errors.New("batas SKS terlampaui")

	var totalSuccess int64
	var totalLimitExceeded int64
	var wg sync.WaitGroup
	startGate := make(chan struct{})

	// Setiap dari 10 mahasiswa mengirim 12 request serentak
	for _, uid := range userIDs {
		for _, c := range courses {
			wg.Add(1)
			go func(userID int, course courseInfo) {
				defer wg.Done()
				<-startGate

				err := txManager.WithTx(ctx, func(tx repository.DBTX) error {
					repo := courseRepo.WithTx(tx)

					// LockStudent mengunci user ini
					if err := repo.LockStudent(ctx, userID); err != nil {
						return err
					}

					taken, err := repo.SumCredits(ctx, userID)
					if err != nil {
						return err
					}
					if !service.WithinCreditLimit(taken, course.credits) {
						return errCreditLimit
					}

					if _, err := repo.IncrementEnrolled(ctx, course.id); err != nil {
						return err
					}
					return repo.InsertEnrollment(ctx, course.id, userID)
				})

				if err == nil {
					atomic.AddInt64(&totalSuccess, 1)
				} else if errors.Is(err, errCreditLimit) {
					atomic.AddInt64(&totalLimitExceeded, 1)
				}
			}(uid, c)
		}
	}

	close(startGate)
	wg.Wait()

	t.Logf("Hasil Uji Batas SKS (10 mahasiswa x 12 kelas 3 SKS = 120 requests):")
	t.Logf("  Berhasil diambil: %d", totalSuccess)
	t.Logf("  Ditolak melebihi SKS: %d", totalLimitExceeded)

	// Periksa SKS setiap mahasiswa di database
	for _, uid := range userIDs {
		var totalCredits int
		err := db.QueryRow(ctx, `
			SELECT COALESCE(SUM(c.credits), 0)
			FROM enrollments e
			JOIN courses c ON c.id = e.course_id
			WHERE e.user_id = $1
		`, uid).Scan(&totalCredits)
		if err != nil {
			t.Fatalf("gagal membaca SKS user %d: %v", uid, err)
		}

		if totalCredits > service.MaxCreditsPerSemester {
			t.Fatalf("Mahasiswa user_id=%d melebihi batas 24 SKS: %d SKS!", uid, totalCredits)
		}
		if totalCredits != 24 {
			t.Errorf("Mahasiswa user_id=%d diharapkan tepat 24 SKS, aktual: %d SKS", uid, totalCredits)
		}
	}
	t.Log("Seluruh 10 mahasiswa <= 24 SKS: KONSISTEN")
}

// TestDuplicateEnrollment menguji pendaftaran ganda ditolak dengan ErrDuplicate dan enrolled_count tidak bertambah
func TestDuplicateEnrollment(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	resetLoadtestData(t, db)

	var courseID int
	err := db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF101'").Scan(&courseID)
	if err != nil {
		t.Fatalf("find IF101: %v", err)
	}

	var userID int
	err = db.QueryRow(ctx, "SELECT id FROM users WHERE username = 'mhs001'").Scan(&userID)
	if err != nil {
		t.Fatalf("find mhs001: %v", err)
	}

	courseRepo := repository.NewCourseRepository(db)
	txManager := repository.NewTxManager(db)

	enroll := func() error {
		return txManager.WithTx(ctx, func(tx repository.DBTX) error {
			repo := courseRepo.WithTx(tx)
			if err := repo.LockStudent(ctx, userID); err != nil {
				return err
			}
			if _, err := repo.IncrementEnrolled(ctx, courseID); err != nil {
				return err
			}
			return repo.InsertEnrollment(ctx, courseID, userID)
		})
	}

	// Pertama: berhasil
	if err := enroll(); err != nil {
		t.Fatalf("pendaftaran pertama gagal: %v", err)
	}

	// Kedua: ditolak ErrDuplicate
	err = enroll()
	if !errors.Is(err, repository.ErrDuplicate) {
		t.Fatalf("pendaftaran kedua diharapkan ErrDuplicate, didapat: %v", err)
	}

	// Periksa enrolled_count di database harus tetap 1
	var enrolledCount int
	_ = db.QueryRow(ctx, "SELECT enrolled_count FROM courses WHERE id = $1", courseID).Scan(&enrolledCount)
	if enrolledCount != 1 {
		t.Fatalf("enrolled_count harus 1, didapat: %d", enrolledCount)
	}
}

// TestDropEnrollment menguji pembatalan kelas dan membuktikan perbaikan Planted Error 7:
// DeleteEnrollment memeriksa RowsAffected == 0 agar ghost drop tidak mengurangi kursi.
func TestDropEnrollment(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	resetLoadtestData(t, db)

	var courseID int
	_ = db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF101'").Scan(&courseID)
	var userID int
	_ = db.QueryRow(ctx, "SELECT id FROM users WHERE username = 'mhs001'").Scan(&userID)

	courseRepo := repository.NewCourseRepository(db)
	txManager := repository.NewTxManager(db)

	// Drop kelas yang belum pernah diambil -> harus ErrNotEnrolled
	dropErr := txManager.WithTx(ctx, func(tx repository.DBTX) error {
		repo := courseRepo.WithTx(tx)
		if err := repo.LockStudent(ctx, userID); err != nil {
			return err
		}
		if err := repo.DeleteEnrollment(ctx, courseID, userID); err != nil {
			return err
		}
		_, err := repo.DecrementEnrolled(ctx, courseID)
		return err
	})

	if !errors.Is(dropErr, repository.ErrNotEnrolled) {
		t.Fatalf("Drop pendaftaran fiktif harus mengembalikan ErrNotEnrolled, didapat: %v", dropErr)
	}

	// Pastikan enrolled_count tetap 0
	var enrolledCount int
	_ = db.QueryRow(ctx, "SELECT enrolled_count FROM courses WHERE id = $1", courseID).Scan(&enrolledCount)
	if enrolledCount != 0 {
		t.Fatalf("enrolled_count harus tetap 0 setelah ghost drop, didapat: %d", enrolledCount)
	}
}

// TestConcurrency_CrossSwap_DeadlockFree menguji 20 pasang mahasiswa (40 mahasiswa)
// saling tukar kelas berlawanan arah secara serentak (IF101 <-> IF102).
// Memverifikasi perbaikan Planted Error 3 (LockCourses), deadlock avoidance, dan Planted Error 8 (Swap inside tx).
func TestConcurrency_CrossSwap_DeadlockFree(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	resetLoadtestData(t, db)

	var id101, id102 int
	_ = db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF101'").Scan(&id101)
	_ = db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF102'").Scan(&id102)

	// Ambil 40 user mahasiswa: 20 pertama didaftarkan ke IF101, 20 berikutnya ke IF102
	rows, err := db.Query(ctx, "SELECT id FROM users WHERE username LIKE 'mhs%' ORDER BY username LIMIT 40")
	if err != nil {
		t.Fatalf("get users: %v", err)
	}
	defer rows.Close()

	var usersA, usersB []int
	idx := 0
	for rows.Next() {
		var uid int
		_ = rows.Scan(&uid)
		if idx < 20 {
			usersA = append(usersA, uid)
		} else {
			usersB = append(usersB, uid)
		}
		idx++
	}

	// Daftarkan kelompok A ke IF101 dan B ke IF102
	for _, uid := range usersA {
		_, _ = db.Exec(ctx, "INSERT INTO enrollments (course_id, user_id) VALUES ($1, $2)", id101, uid)
	}
	for _, uid := range usersB {
		_, _ = db.Exec(ctx, "INSERT INTO enrollments (course_id, user_id) VALUES ($1, $2)", id102, uid)
	}
	_, _ = db.Exec(ctx, "UPDATE courses SET enrolled_count = (SELECT COUNT(*) FROM enrollments WHERE course_id = courses.id) WHERE id IN ($1, $2)", id101, id102)

	courseRepo := repository.NewCourseRepository(db)
	txManager := repository.NewTxManager(db)

	swapFunc := func(userID, fromID, toID int) error {
		return txManager.WithTx(ctx, func(tx repository.DBTX) error {
			repo := courseRepo.WithTx(tx)

			if err := repo.LockStudent(ctx, userID); err != nil {
				return err
			}

			// Kunci kedua kelas dalam urutan ID menaik untuk mencegah deadlock
			if err := repo.LockCourses(ctx, fromID, toID); err != nil {
				return err
			}

			from, err := repo.FindByID(ctx, fromID)
			if err != nil {
				return err
			}
			to, err := repo.FindByID(ctx, toID)
			if err != nil {
				return err
			}

			taken, err := repo.SumCredits(ctx, userID)
			if err != nil {
				return err
			}
			if service.CreditsAfterSwap(taken, from.Credits, to.Credits) > service.MaxCreditsPerSemester {
				return errors.New("credit limit exceeded")
			}

			if err := repo.DeleteEnrollment(ctx, fromID, userID); err != nil {
				return err
			}
			// Memperbaiki Planted Error 8: repo.DecrementEnrolled digunakan di dalam tx
			if _, err := repo.DecrementEnrolled(ctx, fromID); err != nil {
				return err
			}
			if _, err := repo.IncrementEnrolled(ctx, toID); err != nil {
				return err
			}
			return repo.InsertEnrollment(ctx, toID, userID)
		})
	}

	var wg sync.WaitGroup
	var successSwaps int64
	var deadlockErrors int64
	startGate := make(chan struct{})

	// 20 mahasiswa pindah IF101 -> IF102
	for _, uid := range usersA {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-startGate
			err := swapFunc(id, id101, id102)
			if err == nil {
				atomic.AddInt64(&successSwaps, 1)
			} else {
				t.Logf("swap error A: %v", err)
			}
		}(uid)
	}

	// 20 mahasiswa pindah IF102 -> IF101
	for _, uid := range usersB {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-startGate
			err := swapFunc(id, id102, id101)
			if err == nil {
				atomic.AddInt64(&successSwaps, 1)
			} else {
				t.Logf("swap error B: %v", err)
			}
		}(uid)
	}

	close(startGate)
	wg.Wait()

	t.Logf("Hasil Cross-Swap (40 mahasiswa serentak):")
	t.Logf("  Berhasil swap: %d", successSwaps)
	t.Logf("  Deadlock detected: %d", deadlockErrors)

	if successSwaps != 40 {
		t.Errorf("Ekspektasi tepat 40 swap berhasil, aktual: %d", successSwaps)
	}
	if deadlockErrors != 0 {
		t.Errorf("Ekspektasi 0 deadlock, aktual: %d", deadlockErrors)
	}

	// Cek konsistensi
	var count101, count102 int
	_ = db.QueryRow(ctx, "SELECT enrolled_count FROM courses WHERE id = $1", id101).Scan(&count101)
	_ = db.QueryRow(ctx, "SELECT enrolled_count FROM courses WHERE id = $1", id102).Scan(&count102)
	if count101 != 20 || count102 != 20 {
		t.Errorf("Hasil akhir kelas tidak konsisten: IF101=%d, IF102=%d (harus 20)", count101, count102)
	}
}

// TestOptimisticLocking_UpdateQuota menguji optimisitic locking:
// Memverifikasi perbaikan Planted Error 9 (version = version + 1).
func TestOptimisticLocking_UpdateQuota(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	resetLoadtestData(t, db)

	courseRepo := repository.NewCourseRepository(db)

	var c3ID int
	_ = db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF301'").Scan(&c3ID)
	course, err := courseRepo.FindByID(ctx, c3ID)
	if err != nil {
		t.Fatalf("find course IF301: %v", err)
	}

	currentVersion := course.Version
	newQuota := 50

	// 1. Update dengan version yang benar -> Sukses, version naik 1
	updated, err := courseRepo.UpdateQuota(ctx, course.ID, newQuota, currentVersion)
	if err != nil {
		t.Fatalf("UpdateQuota dengan version benar gagal: %v", err)
	}
	if updated.Quota != newQuota {
		t.Errorf("kuota baru diharapkan %d, didapat %d", newQuota, updated.Quota)
	}
	if updated.Version != currentVersion+1 {
		t.Errorf("version baru diharapkan %d, didapat %d (Planted Error 9 fix check)", currentVersion+1, updated.Version)
	}

	// 2. Update dengan version lama (stale version) -> Ditolak ErrVersionConflict
	_, err = courseRepo.UpdateQuota(ctx, course.ID, 60, currentVersion)
	if !errors.Is(err, repository.ErrVersionConflict) {
		t.Fatalf("UpdateQuota dengan version lama diharapkan ErrVersionConflict, didapat: %v", err)
	}

	// 3. Menurunkan kuota di bawah enrolled_count saat ini -> Ditolak ErrQuotaBelowFilled
	// Daftarkan 5 user ke course 1
	var uids []int
	urows, err := db.Query(ctx, "SELECT id FROM users WHERE username LIKE 'mhs%' LIMIT 5")
	if err != nil {
		t.Fatalf("get users: %v", err)
	}
	for urows.Next() {
		var uid int
		_ = urows.Scan(&uid)
		uids = append(uids, uid)
	}
	urows.Close()

	for _, uid := range uids {
		_, _ = db.Exec(ctx, "INSERT INTO enrollments (course_id, user_id) VALUES ($1, $2)", course.ID, uid)
	}
	_, _ = db.Exec(ctx, "UPDATE courses SET enrolled_count = (SELECT COUNT(*) FROM enrollments WHERE course_id = courses.id) WHERE id = $1", course.ID)

	courseAfter, _ := courseRepo.FindByID(ctx, course.ID)
	_, err = courseRepo.UpdateQuota(ctx, course.ID, 3, courseAfter.Version)
	if !errors.Is(err, repository.ErrQuotaBelowFilled) {
		t.Fatalf("UpdateQuota di bawah enrolled_count diharapkan ErrQuotaBelowFilled, didapat: %v", err)
	}

	// Bersihkan kembali data
	resetLoadtestData(t, db)
}
