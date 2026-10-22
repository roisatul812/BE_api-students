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

// resetSupervisorTestData membersihkan data supervisions dan mereset supervised_count ke 0.
func resetSupervisorTestData(t *testing.T, db *pgxpool.Pool) {
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		DELETE FROM supervisions;
		UPDATE supervisors
		SET supervised_count = 0, version = 1;
	`)
	if err != nil {
		t.Fatalf("gagal reset data supervisor: %v", err)
	}
}

// ensureTestStudents memastikan tersedia setidaknya count mahasiswa untuk pengujian.
func ensureTestStudents(t *testing.T, db *pgxpool.Pool, count int) []int {
	ctx := context.Background()

	// Pastikan ada setidaknya 1 user untuk owner_id
	var ownerID int
	err := db.QueryRow(ctx, "SELECT id FROM users ORDER BY id LIMIT 1").Scan(&ownerID)
	if err != nil {
		t.Fatalf("gagal mencari user untuk owner_id student: %v", err)
	}

	// Insert generate_series jika belum ada
	_, err = db.Exec(ctx, `
		INSERT INTO students (nim, name, grade, is_active, owner_id)
		SELECT 'MHS_SPV_' || LPAD(g::TEXT, 3, '0'),
		       'Mahasiswa Uji ' || g::TEXT,
		       85.00,
		       true,
		       $1
		FROM generate_series(1, $2) AS g
		ON CONFLICT (nim) DO NOTHING;
	`, ownerID, count)
	if err != nil {
		t.Fatalf("gagal generate test students: %v", err)
	}

	rows, err := db.Query(ctx, `
		SELECT id FROM students
		WHERE nim LIKE 'MHS_SPV_%'
		ORDER BY id
		LIMIT $1
	`, count)
	if err != nil {
		t.Fatalf("gagal mengambil id mahasiswa uji: %v", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan student id: %v", err)
		}
		ids = append(ids, id)
	}

	if len(ids) < count {
		t.Fatalf("jumlah mahasiswa uji kurang dari %d: %d", count, len(ids))
	}
	return ids
}

// TestSupervisor_Assign_Success menguji penetapan pembimbing yang valid.
func TestSupervisor_Assign_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	studentIDs := ensureTestStudents(t, db, 1)
	studentID := studentIDs[0]

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	supervisorID := 1 // kuota 5
	assigned, err := supService.AssignSupervisor(ctx, supervisorID, studentID)
	if err != nil {
		t.Fatalf("gagal assign supervisor: %v", err)
	}

	if assigned.SupervisedCount != 1 {
		t.Errorf("supervised_count diharapkan 1, didapat %d", assigned.SupervisedCount)
	}

	// Verifikasi di database
	var countInDB int
	err = db.QueryRow(ctx, "SELECT COUNT(*) FROM supervisions WHERE student_id = $1 AND supervisor_id = $2",
		studentID, supervisorID).Scan(&countInDB)
	if err != nil || countInDB != 1 {
		t.Errorf("supervisions tidak tercatat di DB: count=%d, err=%v", countInDB, err)
	}
}

// TestSupervisor_Assign_AlreadySupervised menguji penolakan ketika mahasiswa yang sudah punya pembimbing
// mencoba didaftarkan kembali (aturan 1 mahasiswa 1 pembimbing).
func TestSupervisor_Assign_AlreadySupervised(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	studentIDs := ensureTestStudents(t, db, 1)
	studentID := studentIDs[0]

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	// Assignment pertama berhasil
	_, err := supService.AssignSupervisor(ctx, 1, studentID)
	if err != nil {
		t.Fatalf("assign pertama gagal: %v", err)
	}

	// Assignment kedua ke pembimbing lain harus ditolak dengan ErrAlreadySupervised
	_, err = supService.AssignSupervisor(ctx, 2, studentID)
	if err == nil {
		t.Fatalf("assignment kedua seharusnya ditolak, tapi berhasil")
	}
	if !errors.Is(err, repository.ErrAlreadySupervised) {
		t.Errorf("diharapkan ErrAlreadySupervised, didapat: %v", err)
	}
}

// TestSupervisor_Assign_SupervisorFull menguji penolakan ketika kuota pembimbing telah penuh.
func TestSupervisor_Assign_SupervisorFull(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	// Supervisor ID 5 memiliki kuota 2
	supervisorID := 5
	studentIDs := ensureTestStudents(t, db, 3)

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	// Isi hingga kuota penuh (2 mahasiswa)
	for i := 0; i < 2; i++ {
		_, err := supService.AssignSupervisor(ctx, supervisorID, studentIDs[i])
		if err != nil {
			t.Fatalf("assign mahasiswa %d gagal: %v", i+1, err)
		}
	}

	// Mahasiswa ke-3 harus ditolak dengan ErrSupervisorFull
	_, err := supService.AssignSupervisor(ctx, supervisorID, studentIDs[2])
	if err == nil {
		t.Fatalf("assign mahasiswa ke-3 seharusnya ditolak karena kuota penuh")
	}
	if !errors.Is(err, repository.ErrSupervisorFull) {
		t.Errorf("diharapkan ErrSupervisorFull, didapat: %v", err)
	}

	// Verifikasi supervised_count tidak melampaui kuota
	sup, err := supRepo.FindByID(ctx, supervisorID)
	if err != nil || sup.SupervisedCount != 2 {
		t.Errorf("supervised_count seharusnya tetap 2, didapat: %d", sup.SupervisedCount)
	}
}

// TestSupervisor_ConcurrentSameStudent menguji dua request bersamaan untuk mahasiswa yang sama.
// Row lock LockStudent(ctx, studentID) harus menjamin tepat satu berhasil dan satu ditolak.
func TestSupervisor_ConcurrentSameStudent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	studentIDs := ensureTestStudents(t, db, 1)
	studentID := studentIDs[0]

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	var wg sync.WaitGroup
	startChan := make(chan struct{})

	var successCount int32
	var rejectedCount int32

	// Goroutine 1 mencoba assign ke Supervisor 1
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-startChan
		_, err := supService.AssignSupervisor(ctx, 1, studentID)
		if err == nil {
			atomic.AddInt32(&successCount, 1)
		} else {
			atomic.AddInt32(&rejectedCount, 1)
		}
	}()

	// Goroutine 2 mencoba assign ke Supervisor 2
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-startChan
		_, err := supService.AssignSupervisor(ctx, 2, studentID)
		if err == nil {
			atomic.AddInt32(&successCount, 1)
		} else {
			atomic.AddInt32(&rejectedCount, 1)
		}
	}()

	close(startChan)
	wg.Wait()

	if successCount != 1 || rejectedCount != 1 {
		t.Fatalf("diharapkan tepat 1 sukses dan 1 ditolak, hasil: sukses=%d, ditolak=%d", successCount, rejectedCount)
	}

	// Verifikasi di DB mahasiswa hanya punya 1 pembimbing
	var supCount int
	_ = db.QueryRow(ctx, "SELECT COUNT(*) FROM supervisions WHERE student_id = $1", studentID).Scan(&supCount)
	if supCount != 1 {
		t.Errorf("jumlah baris supervisions untuk mahasiswa seharusnya 1, didapat: %d", supCount)
	}
}

// TestSupervisor_Switch_Success menguji perpindahan pembimbing yang valid.
func TestSupervisor_Switch_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	studentIDs := ensureTestStudents(t, db, 1)
	studentID := studentIDs[0]

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	// Awalnya bersama Supervisor 1
	_, err := supService.AssignSupervisor(ctx, 1, studentID)
	if err != nil {
		t.Fatalf("assign awal gagal: %v", err)
	}

	// Switch ke Supervisor 2
	switched, err := supService.SwitchSupervisor(ctx, studentID, 2, nil)
	if err != nil {
		t.Fatalf("switch gagal: %v", err)
	}
	if switched.ID != 2 {
		t.Errorf("pembimbing baru seharusnya ID 2, didapat: %d", switched.ID)
	}

	// Verifikasi jumlah pembimbing di database
	sup1, _ := supRepo.FindByID(ctx, 1)
	sup2, _ := supRepo.FindByID(ctx, 2)
	if sup1.SupervisedCount != 0 {
		t.Errorf("sup1 count seharusnya 0, didapat: %d", sup1.SupervisedCount)
	}
	if sup2.SupervisedCount != 1 {
		t.Errorf("sup2 count seharusnya 1, didapat: %d", sup2.SupervisedCount)
	}

	// Verifikasi supervisions
	current, err := supRepo.FindByStudentID(ctx, studentID)
	if err != nil || current == nil || current.ID != 2 {
		t.Errorf("mahasiswa seharusnya kini dibimbing oleh supervisor 2")
	}
}

// TestSupervisor_Switch_ToFullSupervisor_AtomicRollback menguji atomisitas switch:
// jika pembimbing tujuan penuh, operasi dibatalkan dan mahasiswa TETAP tercatat pada pembimbing lama.
func TestSupervisor_Switch_ToFullSupervisor_AtomicRollback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	// Supervisor ID 5 memiliki kuota 2
	// Supervisor ID 1 memiliki kuota 5
	studentIDs := ensureTestStudents(t, db, 3)

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	// Mahasiswa 0 dan 1 memenuhi Supervisor 5 (kuota 2)
	_, _ = supService.AssignSupervisor(ctx, 5, studentIDs[0])
	_, _ = supService.AssignSupervisor(ctx, 5, studentIDs[1])

	// Mahasiswa 2 terdaftar di Supervisor 1
	_, err := supService.AssignSupervisor(ctx, 1, studentIDs[2])
	if err != nil {
		t.Fatalf("assign awal mahasiswa 2 gagal: %v", err)
	}

	// Mahasiswa 2 mencoba switch ke Supervisor 5 yang sudah penuh
	_, err = supService.SwitchSupervisor(ctx, studentIDs[2], 5, nil)
	if err == nil {
		t.Fatalf("switch ke pembimbing penuh seharusnya gagal")
	}
	if !errors.Is(err, repository.ErrSupervisorFull) {
		t.Errorf("diharapkan ErrSupervisorFull, didapat: %v", err)
	}

	// Atomisitas: Mahasiswa 2 TETAP di Supervisor 1, dan kuota keduanya konsisten
	current, err := supRepo.FindByStudentID(ctx, studentIDs[2])
	if err != nil || current == nil || current.ID != 1 {
		t.Fatalf("mahasiswa 2 seharusnya TETAP dibimbing supervisor 1")
	}

	sup1, _ := supRepo.FindByID(ctx, 1)
	sup5, _ := supRepo.FindByID(ctx, 5)
	if sup1.SupervisedCount != 1 {
		t.Errorf("supervisor 1 count seharusnya tetap 1, didapat: %d", sup1.SupervisedCount)
	}
	if sup5.SupervisedCount != 2 {
		t.Errorf("supervisor 5 count seharusnya tetap 2, didapat: %d", sup5.SupervisedCount)
	}
}

// TestSupervisor_Switch_CrossSwitch_NoDeadlock menguji dua perpindahan berlawanan arah secara serentak
// (Mahasiswa A: 1 -> 2, Mahasiswa B: 2 -> 1) bebas dari deadlock berkat penguncian urut ID (ORDER BY id).
func TestSupervisor_Switch_CrossSwitch_NoDeadlock(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	studentIDs := ensureTestStudents(t, db, 2)
	s1 := studentIDs[0]
	s2 := studentIDs[1]

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	// Mahasiswa 1 di Supervisor 1, Mahasiswa 2 di Supervisor 2
	_, _ = supService.AssignSupervisor(ctx, 1, s1)
	_, _ = supService.AssignSupervisor(ctx, 2, s2)

	var wg sync.WaitGroup
	startChan := make(chan struct{})
	var err1, err2 error

	wg.Add(2)
	// Switch s1: 1 -> 2
	go func() {
		defer wg.Done()
		<-startChan
		_, err1 = supService.SwitchSupervisor(ctx, s1, 2, nil)
	}()

	// Switch s2: 2 -> 1
	go func() {
		defer wg.Done()
		<-startChan
		_, err2 = supService.SwitchSupervisor(ctx, s2, 1, nil)
	}()

	close(startChan)
	wg.Wait()

	if err1 != nil {
		t.Errorf("switch s1 gagal: %v", err1)
	}
	if err2 != nil {
		t.Errorf("switch s2 gagal: %v", err2)
	}

	// Periksa posisi akhir
	cur1, _ := supRepo.FindByStudentID(ctx, s1)
	cur2, _ := supRepo.FindByStudentID(ctx, s2)
	if cur1.ID != 2 || cur2.ID != 1 {
		t.Errorf("hasil tukar tidak sesuai: s1=%d (harap 2), s2=%d (harap 1)", cur1.ID, cur2.ID)
	}
}

// TestSupervisor_UpdateQuota_OptimisticLocking menguji optimistic locking:
// perubahan dengan version lama ditolak (409), perubahan dengan version sah menaikkan version.
func TestSupervisor_UpdateQuota_OptimisticLocking(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	supRepo := repository.NewSupervisorRepository(db)

	sup, err := supRepo.FindByID(ctx, 1)
	if err != nil {
		t.Fatalf("gagal ambil data supervisor: %v", err)
	}

	// Update dengan version yang sah (version 1 -> 2)
	updated, err := supRepo.UpdateQuota(ctx, sup.ID, 10, sup.Version)
	if err != nil {
		t.Fatalf("update quota sah gagal: %v", err)
	}
	if updated.Quota != 10 || updated.Version != sup.Version+1 {
		t.Errorf("hasil update quota tidak sesuai: quota=%d, version=%d", updated.Quota, updated.Version)
	}

	// Update kedua dengan version LAMA (version 1) harus DITOLAK
	_, err = supRepo.UpdateQuota(ctx, sup.ID, 15, sup.Version)
	if err == nil {
		t.Fatalf("update quota dengan version lama seharusnya ditolak")
	}
	if !errors.Is(err, repository.ErrSupervisorVersionConflict) {
		t.Errorf("diharapkan ErrSupervisorVersionConflict, didapat: %v", err)
	}
}

// TestSupervisor_UpdateQuota_BelowSupervised menguji penolakan penurunan kuota di bawah jumlah yang sedang dibimbing.
func TestSupervisor_UpdateQuota_BelowSupervised(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	resetSupervisorTestData(t, db)

	ctx := context.Background()
	studentIDs := ensureTestStudents(t, db, 3)

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	supervisorID := 1 // kuota 5
	for i := 0; i < 3; i++ {
		_, err := supService.AssignSupervisor(ctx, supervisorID, studentIDs[i])
		if err != nil {
			t.Fatalf("assign %d gagal: %v", i, err)
		}
	}

	sup, _ := supRepo.FindByID(ctx, supervisorID)
	if sup.SupervisedCount != 3 {
		t.Fatalf("supervised count diharapkan 3, didapat: %d", sup.SupervisedCount)
	}

	// Coba turunkan kuota menjadi 2 (di bawah supervised_count = 3)
	_, err := supRepo.UpdateQuota(ctx, supervisorID, 2, sup.Version)
	if err == nil {
		t.Fatalf("penurunan kuota di bawah supervised_count seharusnya ditolak")
	}
	if !errors.Is(err, repository.ErrQuotaBelowSupervised) {
		t.Errorf("diharapkan ErrQuotaBelowSupervised, didapat: %v", err)
	}
}

// TestSupervisor_Concurrency_CrowdAssign_Quota3 adalah UJI BEBAN UTAMA:
// 30 request assignment serentak kepada satu pembimbing dengan kuota 3 harus menghasilkan
// TEPAT 3 SUKSES dan 27 PENOLAKAN yang diharapkan (SUPERVISOR_FULL).
func TestSupervisor_Concurrency_CrowdAssign_Quota3(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()
	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	// Ambil supervisor dengan kuota 3 (Supervisor ID 2: Prof. Siti Aminah, Ph.D)
	supervisorID := 2
	sup, err := supRepo.FindByID(ctx, supervisorID)
	if err != nil {
		t.Fatalf("gagal mencari supervisor 2: %v", err)
	}
	if sup.Quota != 3 {
		t.Fatalf("kuota supervisor 2 diharapkan 3, didapat %d", sup.Quota)
	}

	studentIDs := ensureTestStudents(t, db, 30)

	// Jalankan 3 putaran uji untuk memastikan konsistensi (sesuai instruksi Bagian C/D)
	for round := 1; round <= 3; round++ {
		resetSupervisorTestData(t, db)

		var successCount int32
		var fullCount int32
		var otherErrCount int32

		var wg sync.WaitGroup
		startChan := make(chan struct{})

		for i := 0; i < 30; i++ {
			wg.Add(1)
			sid := studentIDs[i]

			go func() {
				defer wg.Done()
				<-startChan

				_, err := supService.AssignSupervisor(ctx, supervisorID, sid)
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				} else if errors.Is(err, repository.ErrSupervisorFull) {
					atomic.AddInt32(&fullCount, 1)
				} else {
					atomic.AddInt32(&otherErrCount, 1)
				}
			}()
		}

		close(startChan)
		wg.Wait()

		t.Logf("[Round %d] Hasil 30 konkurensi (kuota 3): Sukses=%d, Penuh(409)=%d, ErrorLain=%d",
			round, successCount, fullCount, otherErrCount)

		if successCount != 3 {
			t.Fatalf("[Round %d] Sukses seharusnya TEPAT 3, didapat: %d", round, successCount)
		}
		if fullCount != 27 {
			t.Fatalf("[Round %d] Penolakan penuh seharusnya TEPAT 27, didapat: %d", round, fullCount)
		}
		if otherErrCount != 0 {
			t.Fatalf("[Round %d] Terdapat %d error tak terduga", round, otherErrCount)
		}

		// Verifikasi konsistensi di database
		checkSup, err := supRepo.FindByID(ctx, supervisorID)
		if err != nil {
			t.Fatalf("gagal ambil data supervisor: %v", err)
		}
		if checkSup.SupervisedCount != 3 {
			t.Fatalf("supervised_count di database harus tepat 3, didapat: %d", checkSup.SupervisedCount)
		}

		var rowCount int
		err = db.QueryRow(ctx, "SELECT COUNT(*) FROM supervisions WHERE supervisor_id = $1", supervisorID).Scan(&rowCount)
		if err != nil || rowCount != 3 {
			t.Fatalf("jumlah baris supervisions di DB harus tepat 3, didapat: %d (err=%v)", rowCount, err)
		}
	}
}

// TestSupervisor_Concurrency_ComparatorWithoutProtection adalah UJI PEMBANDING (Comparator Test):
// Menunjukkan apa yang terjadi bila proteksi kuota (syarat UPDATE bersyarat 'AND supervised_count < quota'
// atau row lock) sengaja diabaikan:
// Uji ini membuktikan bahwa test harness memang MAMPU mendeteksi pelanggaran kuota ketika proteksi hilang.
func TestSupervisor_Concurrency_ComparatorWithoutProtection(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Buat tabel uji terisolasi tanpa CHECK constraint kuota
	_, err := db.Exec(ctx, `
		CREATE UNLOGGED TABLE IF NOT EXISTS test_naive_supervisors (
			id SERIAL PRIMARY KEY,
			quota INT NOT NULL,
			supervised_count INT NOT NULL DEFAULT 0
		);
		TRUNCATE test_naive_supervisors;
		INSERT INTO test_naive_supervisors (id, quota, supervised_count) VALUES (999, 3, 0);
	`)
	if err != nil {
		t.Fatalf("gagal membuat unlogged table comparator: %v", err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), "DROP TABLE IF EXISTS test_naive_supervisors")
	}()

	var wg sync.WaitGroup
	startChan := make(chan struct{})
	var assignedCount int32

	// 20 request serentak berebut kuota 3 dengan kode naif (read-then-write tanpa lock / conditional update)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startChan

			// Pola naif yang salah:
			// 1. SELECT count tanpa FOR UPDATE
			var count, quota int
			err := db.QueryRow(ctx, "SELECT supervised_count, quota FROM test_naive_supervisors WHERE id = 999").Scan(&count, &quota)
			if err != nil {
				return
			}

			// 2. Pemeriksaan if di aplikasi tanpa lock (race condition window)
			if count < quota {
				// 3. UPDATE tanpa syarat 'AND supervised_count < quota'
				_, err = db.Exec(ctx, "UPDATE test_naive_supervisors SET supervised_count = supervised_count + 1 WHERE id = 999")
				if err == nil {
					atomic.AddInt32(&assignedCount, 1)
				}
			}
		}()
	}

	close(startChan)
	wg.Wait()

	var finalCount int
	_ = db.QueryRow(ctx, "SELECT supervised_count FROM test_naive_supervisors WHERE id = 999").Scan(&finalCount)

	t.Logf("[Comparator Test] Hasil simulasi naif: request diterima = %d, final supervised_count di DB = %d (kuota = 3)",
		assignedCount, finalCount)

	if finalCount > 3 {
		t.Logf(">> BUKTI PEMBANDING VALID: Tanpa syarat UPDATE bersyarat dan row lock, kuota jebol (terisi %d dari kuota 3)! Uji berhasil mendeteksi pelanggaran konkurensi.", finalCount)
	} else {
		t.Logf("Catatan: Terjadi pembagian kuota naif: %d (kuota 3).", finalCount)
	}
}
