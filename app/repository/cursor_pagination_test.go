package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	model "api-students/app/model"
	"api-students/app/repository"
	"api-students/database"
	"api-students/helper"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	_ = godotenv.Load("../../.env")
	ctx := context.Background()
	db, err := database.Connect(ctx)
	if err != nil {
		t.Skipf("database tidak tersedia: %v", err)
	}
	return db
}

// PlantedFindByCursor merekonstruksi planted error Langkah 7 Modul 7,
// di mana ordering query menggunakan ORDER BY created_at ASC, id ASC (urutan terbalik/tidak stabil untuk feed descending).
func PlantedFindByCursor(ctx context.Context, db *pgxpool.Pool, cursor *model.CursorParam, limit int) ([]model.Student, error) {
	fetchLimit := limit
	query := `
		SELECT id, nim, name, grade, is_active, owner_id, created_at
		FROM students
		ORDER BY created_at ASC, id ASC
		LIMIT $1
	`
	rows, err := db.Query(ctx, query, fetchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var students []model.Student
	for rows.Next() {
		var s model.Student
		if err := rows.Scan(&s.ID, &s.NIM, &s.Name, &s.Grade, &s.IsActive, &s.OwnerID, &s.CreatedAt); err != nil {
			return nil, err
		}
		students = append(students, s)
	}
	return students, nil
}

func TestCursorPagination_PlantedVsFixed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo := repository.NewPostgresStudentRepository(db)

	// Pastikan ada data minimal 5 record untuk testing
	students, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("gagal mengambil data students: %v", err)
	}
	if len(students) < 3 {
		// Insert beberapa student dummy
		for i := 1; i <= 3; i++ {
			dummy := &model.Student{
				NIM:      fmt.Sprintf("TESTNIM%d%d", time.Now().Unix(), i),
				Name:     fmt.Sprintf("Test Student %d", i),
				Grade:    85,
				IsActive: true,
				OwnerID:  1,
			}
			_ = repo.Create(ctx, dummy)
		}
	}

	// 1. REPRODUKSI PLANTED ERROR BUG #8:
	// Query dengan ORDER BY created_at ASC menghasilkan urutan data tertua ke terbaru,
	// bertentangan dengan spesifikasi Modul 7 (feed harus newest first: DESC, DESC).
	plantedList, err := PlantedFindByCursor(ctx, db, nil, 2)
	if err != nil {
		t.Fatalf("planted query error: %v", err)
	}
	t.Logf("PLANTED ERROR REPRODUKSI (ORDER BY ASC, ASC):")
	for i, s := range plantedList {
		t.Logf("  [%d] ID=%d NIM=%s CreatedAt=%s (oldest first)", i+1, s.ID, s.NIM, s.CreatedAt.Format(time.RFC3339))
	}
	if len(plantedList) >= 2 && plantedList[0].CreatedAt.After(plantedList[1].CreatedAt) {
		t.Errorf("planted query seharusnya ASC")
	}

	// 2. PEMBUKTIAN PERBAIKAN BUG #8 (ORDER BY DESC, DESC):
	// Page 1: Ambil 2 record terbaru
	page1, hasMore1, nextCursor1, err := repo.FindByCursor(ctx, nil, 2)
	if err != nil {
		t.Fatalf("fixed FindByCursor page 1 error: %v", err)
	}
	t.Logf("FIXED BUG #8 - PAGE 1 (ORDER BY DESC, DESC):")
	for i, s := range page1 {
		t.Logf("  [%d] ID=%d NIM=%s CreatedAt=%s", i+1, s.ID, s.NIM, s.CreatedAt.Format(time.RFC3339))
	}
	t.Logf("  has_more=%v, next_cursor=%s", hasMore1, nextCursor1)

	if len(page1) >= 2 && page1[0].CreatedAt.Before(page1[1].CreatedAt) {
		t.Errorf("FIXED query harus DESC (newest first)")
	}
	if !hasMore1 || nextCursor1 == "" {
		t.Fatalf("expected hasMore=true and valid nextCursor")
	}

	// 3. SISIPKAN DATA BARU DI ANTARA PAGE 1 DAN PAGE 2
	newStudent := &model.Student{
		NIM:      fmt.Sprintf("NIMNEW%d", time.Now().UnixNano()%1000000000),
		Name:     "Murid Baru Antara Page",
		Grade:    90,
		IsActive: true,
		OwnerID:  1,
	}
	if err := repo.Create(ctx, newStudent); err != nil {
		t.Fatalf("gagal insert murid baru: %v", err)
	}
	defer func() {
		_ = repo.Delete(ctx, newStudent.ID)
	}()
	t.Logf("DATA BARU DISISIPKAN: ID=%d NIM=%s CreatedAt=%s", newStudent.ID, newStudent.NIM, newStudent.CreatedAt.Format(time.RFC3339))

	// 4. REQUEST PAGE 2 MENGGUNAKAN CURSOR PAGE 1
	cParam, err := helper.DecodeCursor(nextCursor1)
	if err != nil {
		t.Fatalf("gagal decode cursor: %v", err)
	}
	param := &model.CursorParam{
		CreatedAt: cParam.CreatedAt,
		ID:        cParam.ID,
	}

	page2, hasMore2, nextCursor2, err := repo.FindByCursor(ctx, param, 2)
	if err != nil {
		t.Fatalf("fixed FindByCursor page 2 error: %v", err)
	}
	t.Logf("FIXED BUG #8 - PAGE 2 (DENGAN CURSOR PAGE 1):")
	for i, s := range page2 {
		t.Logf("  [%d] ID=%d NIM=%s CreatedAt=%s", i+1, s.ID, s.NIM, s.CreatedAt.Format(time.RFC3339))
	}
	t.Logf("  has_more=%v, next_cursor=%s", hasMore2, nextCursor2)

	// Pastikan tidak ada duplikasi antara page 1 dan page 2
	page1IDs := make(map[int]bool)
	for _, s := range page1 {
		page1IDs[s.ID] = true
	}
	for _, s := range page2 {
		if page1IDs[s.ID] {
			t.Errorf("DUPLIKASI TERDETEKSI: ID %d muncul di page 1 dan page 2!", s.ID)
		}
		if s.ID == newStudent.ID {
			t.Errorf("DATA BARU BOCOR KE PAGE 2: ID %d seharusnya di atas page 1!", s.ID)
		}
	}

	// 5. TEST LAST PAGE (dengan limit besar atau cursor menuju akhir)
	largePage, hasMoreLarge, nextCursorLarge, err := repo.FindByCursor(ctx, nil, 1000)
	if err != nil {
		t.Fatalf("last page error: %v", err)
	}
	t.Logf("LAST PAGE CHECK: total=%d, has_more=%v, next_cursor=%q", len(largePage), hasMoreLarge, nextCursorLarge)
	if hasMoreLarge {
		t.Errorf("expected has_more=false for fetch all")
	}
	if nextCursorLarge != "" {
		t.Errorf("expected next_cursor empty for last page")
	}
}
