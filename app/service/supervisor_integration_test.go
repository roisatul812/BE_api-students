package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/app/service"
	"api-students/config"
	"api-students/database"
	"api-students/helper"
	"api-students/route"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type supervisorTestEnv struct {
	app        *fiber.App
	db         *pgxpool.Pool
	jwtManager *helper.JWTManager
}

func setupSupervisorTestApp(t *testing.T) *supervisorTestEnv {
	_ = godotenv.Load("../../.env")
	ctx := context.Background()
	db, err := database.Connect(ctx)
	if err != nil {
		t.Skipf("database tidak tersedia: %v", err)
	}

	roleRepo := repository.NewRoleRepository(db)
	perms, err := roleRepo.LoadPermissions(ctx)
	if err != nil {
		t.Fatalf("load perms: %v", err)
	}
	permSet := helper.NewPermissionSet(perms)

	jwtManager := helper.NewJWTManager("test_secret_for_modul9_testing_key_32bytes!", "test-issuer", 1*time.Hour)

	supRepo := repository.NewSupervisorRepository(db)
	txManager := repository.NewTxManager(db)
	supService := service.NewSupervisorService(supRepo, txManager)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	app := fiber.New(fiber.Config{
		ErrorHandler: config.NewErrorHandler(logger),
	})

	route.SupervisorRoutes(app, supService, jwtManager, permSet)

	return &supervisorTestEnv{
		app:        app,
		db:         db,
		jwtManager: jwtManager,
	}
}

func resetSupervisorTestDataInService(t *testing.T, db *pgxpool.Pool) {
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

func ensureServiceTestStudents(t *testing.T, db *pgxpool.Pool, count int) []int {
	ctx := context.Background()
	var ownerID int
	err := db.QueryRow(ctx, "SELECT id FROM users ORDER BY id LIMIT 1").Scan(&ownerID)
	if err != nil {
		t.Fatalf("gagal mencari user untuk owner_id student: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO students (nim, name, grade, is_active, owner_id)
		SELECT 'MHS_INT_' || LPAD(g::TEXT, 3, '0'),
		       'Mahasiswa Integrasi ' || g::TEXT,
		       90.00,
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
		WHERE nim LIKE 'MHS_INT_%'
		ORDER BY id
		LIMIT $1
	`, count)
	if err != nil {
		t.Fatalf("gagal mengambil id mahasiswa integrasi: %v", err)
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
	return ids
}

func TestHTTP_Supervisor_Assign_Success(t *testing.T) {
	env := setupSupervisorTestApp(t)
	defer env.db.Close()
	resetSupervisorTestDataInService(t, env.db)

	studentIDs := ensureServiceTestStudents(t, env.db, 1)
	studentID := studentIDs[0]

	adminToken := createTestToken(t, env.jwtManager, 1, "admin", "admin")

	body, _ := json.Marshal(model.AssignSupervisorRequest{
		StudentID: studentID,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/supervisors/1/assign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 Created, got %d, body: %s", resp.StatusCode, string(respBody))
	}
}

func TestHTTP_Supervisor_Assign_Forbidden_WithoutPermission(t *testing.T) {
	env := setupSupervisorTestApp(t)
	defer env.db.Close()
	resetSupervisorTestDataInService(t, env.db)

	studentIDs := ensureServiceTestStudents(t, env.db, 1)
	studentID := studentIDs[0]

	// User biasa (role user) tidak memiliki permission supervision:manage
	userToken := createTestToken(t, env.jwtManager, 2, "regular_user", "user")

	body, _ := json.Marshal(model.AssignSupervisorRequest{
		StudentID: studentID,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/supervisors/1/assign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)

	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", resp.StatusCode)
	}
}

func TestHTTP_Supervisor_Assign_AlreadySupervised(t *testing.T) {
	env := setupSupervisorTestApp(t)
	defer env.db.Close()
	resetSupervisorTestDataInService(t, env.db)

	studentIDs := ensureServiceTestStudents(t, env.db, 1)
	studentID := studentIDs[0]

	adminToken := createTestToken(t, env.jwtManager, 1, "admin", "admin")

	body, _ := json.Marshal(model.AssignSupervisorRequest{
		StudentID: studentID,
	})

	// Assign pertama
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/supervisors/1/assign", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+adminToken)
	resp1, _ := env.app.Test(req1)
	resp1.Body.Close()

	// Assign kedua untuk mahasiswa yang sama
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/supervisors/2/assign", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	resp2, _ := env.app.Test(req2)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", resp2.StatusCode)
	}

	var errResp map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&errResp)
	if errResp["code"] != "ALREADY_SUPERVISED" {
		t.Errorf("expected error code ALREADY_SUPERVISED, got %v", errResp["code"])
	}
}

func TestHTTP_Supervisor_Switch_Success(t *testing.T) {
	env := setupSupervisorTestApp(t)
	defer env.db.Close()
	resetSupervisorTestDataInService(t, env.db)

	studentIDs := ensureServiceTestStudents(t, env.db, 1)
	studentID := studentIDs[0]

	adminToken := createTestToken(t, env.jwtManager, 1, "admin", "admin")

	// Assign ke Supervisor 3
	assignBody, _ := json.Marshal(model.AssignSupervisorRequest{StudentID: studentID})
	reqAssign := httptest.NewRequest(http.MethodPost, "/api/v1/supervisors/3/assign", bytes.NewReader(assignBody))
	reqAssign.Header.Set("Content-Type", "application/json")
	reqAssign.Header.Set("Authorization", "Bearer "+adminToken)
	respA, _ := env.app.Test(reqAssign)
	respA.Body.Close()

	// Switch ke Supervisor 4
	switchBody, _ := json.Marshal(model.SwitchSupervisorRequest{
		StudentID:      studentID,
		ToSupervisorID: 4,
	})
	reqSwitch := httptest.NewRequest(http.MethodPost, "/api/v1/supervisions/switch", bytes.NewReader(switchBody))
	reqSwitch.Header.Set("Content-Type", "application/json")
	reqSwitch.Header.Set("Authorization", "Bearer "+adminToken)

	respS, err := env.app.Test(reqSwitch)
	if err != nil {
		t.Fatalf("request switch failed: %v", err)
	}
	defer respS.Body.Close()

	if respS.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(respS.Body)
		t.Fatalf("expected 200 OK, got %d, body: %s", respS.StatusCode, string(respBody))
	}
}

func TestHTTP_Supervisor_UpdateQuota_OptimisticLocking(t *testing.T) {
	env := setupSupervisorTestApp(t)
	defer env.db.Close()
	resetSupervisorTestDataInService(t, env.db)

	adminToken := createTestToken(t, env.jwtManager, 1, "admin", "admin")

	// Update quota dengan version 1 (sah)
	bodyValid, _ := json.Marshal(model.UpdateSupervisorQuotaRequest{
		Quota:   8,
		Version: 1,
	})
	reqValid := httptest.NewRequest(http.MethodPatch, "/api/v1/supervisors/1", bytes.NewReader(bodyValid))
	reqValid.Header.Set("Content-Type", "application/json")
	reqValid.Header.Set("Authorization", "Bearer "+adminToken)

	respValid, err := env.app.Test(reqValid)
	if err != nil {
		t.Fatalf("patch valid failed: %v", err)
	}
	defer respValid.Body.Close()

	if respValid.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(respValid.Body)
		t.Fatalf("expected 200 OK, got %d, body: %s", respValid.StatusCode, string(respBody))
	}

	// Update quota dengan version 1 lagi (stale version) harus ditolak 409 VERSION_CONFLICT
	bodyStale, _ := json.Marshal(model.UpdateSupervisorQuotaRequest{
		Quota:   10,
		Version: 1,
	})
	reqStale := httptest.NewRequest(http.MethodPatch, "/api/v1/supervisors/1", bytes.NewReader(bodyStale))
	reqStale.Header.Set("Content-Type", "application/json")
	reqStale.Header.Set("Authorization", "Bearer "+adminToken)

	respStale, err := env.app.Test(reqStale)
	if err != nil {
		t.Fatalf("patch stale failed: %v", err)
	}
	defer respStale.Body.Close()

	if respStale.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", respStale.StatusCode)
	}

	var errResp map[string]interface{}
	_ = json.NewDecoder(respStale.Body).Decode(&errResp)
	if errResp["code"] != "VERSION_CONFLICT" {
		t.Errorf("expected error code VERSION_CONFLICT, got %v", errResp["code"])
	}
}

func TestHTTP_Supervisor_UpdateQuota_BelowSupervised(t *testing.T) {
	env := setupSupervisorTestApp(t)
	defer env.db.Close()
	resetSupervisorTestDataInService(t, env.db)

	studentIDs := ensureServiceTestStudents(t, env.db, 2)
	adminToken := createTestToken(t, env.jwtManager, 1, "admin", "admin")

	// Assign 2 mahasiswa ke supervisor 1
	for _, sid := range studentIDs {
		assignBody, _ := json.Marshal(model.AssignSupervisorRequest{StudentID: sid})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/supervisors/1/assign", bytes.NewReader(assignBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		r, _ := env.app.Test(req)
		r.Body.Close()
	}

	// Coba turunkan kuota menjadi 1 (di bawah supervised_count = 2)
	patchBody, _ := json.Marshal(model.UpdateSupervisorQuotaRequest{
		Quota:   1,
		Version: 1,
	})
	reqPatch := httptest.NewRequest(http.MethodPatch, "/api/v1/supervisors/1", bytes.NewReader(patchBody))
	reqPatch.Header.Set("Content-Type", "application/json")
	reqPatch.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := env.app.Test(reqPatch)
	if err != nil {
		t.Fatalf("patch failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409 Conflict, got %d, body: %s", resp.StatusCode, string(respBody))
	}

	var errResp map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp["code"] != "QUOTA_BELOW_SUPERVISED" {
		t.Errorf("expected error code QUOTA_BELOW_SUPERVISED, got %v", errResp["code"])
	}
}
