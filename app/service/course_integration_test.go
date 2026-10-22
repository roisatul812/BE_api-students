package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

type testEnv struct {
	app        *fiber.App
	db         *pgxpool.Pool
	jwtManager *helper.JWTManager
}

func setupTestApp(t *testing.T) *testEnv {
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

	courseRepo := repository.NewCourseRepository(db)
	txManager := repository.NewTxManager(db)
	courseService := service.NewCourseService(courseRepo, txManager)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	app := fiber.New(fiber.Config{
		ErrorHandler: config.NewErrorHandler(logger),
	})

	route.CourseRoutes(app, courseService, jwtManager, permSet)

	return &testEnv{
		app:        app,
		db:         db,
		jwtManager: jwtManager,
	}
}

func createTestToken(t *testing.T, jwtManager *helper.JWTManager, userID int, username, role string) string {
	token, err := jwtManager.GenerateAccess(model.User{
		ID:       userID,
		Username: username,
		Role:     role,
	})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return token
}

func doRequest(app *fiber.App, method, url, token string, body any) (*http.Response, map[string]any, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req := httptest.NewRequest(method, url, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := app.Test(req, 10000)
	if err != nil {
		return nil, nil, err
	}

	respBytes, _ := io.ReadAll(resp.Body)
	var respMap map[string]any
	_ = json.Unmarshal(respBytes, &respMap)

	return resp, respMap, nil
}

// TestCourseHTTP_Functional Scenarios matching Modul 9 Section C.4
func TestCourseHTTP_Functional(t *testing.T) {
	env := setupTestApp(t)
	defer env.db.Close()

	ctx := context.Background()

	// Gunakan user dan course khusus agar tidak terjadi race condition saat paket repository & service diuji bersamaan
	var studentID int
	err := env.db.QueryRow(ctx, `
		INSERT INTO users (username, email, password, role)
		VALUES ('http_tester_user', 'http_tester@student.unair.ac.id', '!', 'user')
		ON CONFLICT (username) DO UPDATE SET role = 'user'
		RETURNING id;
	`).Scan(&studentID)
	if err != nil {
		t.Fatalf("create/get http_tester_user: %v", err)
	}

	studentToken := createTestToken(t, env.jwtManager, studentID, "http_tester_user", "user")
	adminToken := createTestToken(t, env.jwtManager, 99999, "admin_test", "admin")

	// Gunakan IF307, IF308, IF309 untuk pengujian HTTP terisolasi
	var idSource, idTarget, idFull int
	_ = env.db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF307'").Scan(&idSource)
	_ = env.db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF308'").Scan(&idTarget)
	_ = env.db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF309'").Scan(&idFull)

	// Reset hanya pendaftaran untuk studentID dan courses ini
	_, _ = env.db.Exec(ctx, `
		DELETE FROM enrollments WHERE user_id = $1 OR course_id IN ($2, $3, $4);
		UPDATE courses SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = courses.id)
		WHERE id IN ($2, $3, $4);
	`, studentID, idSource, idTarget, idFull)

	defer func() {
		_, _ = env.db.Exec(ctx, `
			DELETE FROM enrollments WHERE user_id = $1 OR course_id IN ($2, $3, $4);
			UPDATE courses SET enrolled_count = (SELECT COUNT(*) FROM enrollments e WHERE e.course_id = courses.id)
			WHERE id IN ($2, $3, $4);
		`, studentID, idSource, idTarget, idFull)
	}()

	// 1. GET /courses (list courses)
	t.Run("GET /courses - Success", func(t *testing.T) {
		resp, body, err := doRequest(env.app, "GET", "/api/v1/courses", studentToken, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d (err: %v)", resp.StatusCode, err)
		}
		if body["success"] != true {
			t.Errorf("expected success=true, got %v", body["success"])
		}
	})

	// 2. GET /courses/mine (mine empty)
	t.Run("GET /courses/mine - Empty", func(t *testing.T) {
		resp, body, err := doRequest(env.app, "GET", "/api/v1/courses/mine", studentToken, nil)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		data := body["data"].(map[string]any)
		if data["total_credits"].(float64) != 0 {
			t.Errorf("expected total_credits 0, got %v", data["total_credits"])
		}
		if data["max_credits"].(float64) != 24 {
			t.Errorf("expected max_credits 24, got %v", data["max_credits"])
		}
	})

	// 3. POST /courses/:id/enroll - First time -> 201 OK
	t.Run("Enroll into empty course - 201", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/courses/%d/enroll", idSource)
		resp, body, err := doRequest(env.app, "POST", url, studentToken, nil)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d (body: %v)", resp.StatusCode, body)
		}
		if body["success"] != true {
			t.Errorf("expected success=true, got %v", body["success"])
		}
	})

	// 4. POST /courses/:id/enroll - Second time -> 409 ALREADY_ENROLLED
	t.Run("Enroll duplicate - 409 ALREADY_ENROLLED", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/courses/%d/enroll", idSource)
		resp, body, err := doRequest(env.app, "POST", url, studentToken, nil)
		if err != nil || resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d (body: %v)", resp.StatusCode, body)
		}
		if body["code"] != "ALREADY_ENROLLED" {
			t.Errorf("expected code ALREADY_ENROLLED, got %v", body["code"])
		}
	})

	// 5. DELETE /courses/:id/enroll - Drop un-enrolled course -> 409 NOT_ENROLLED
	t.Run("Drop un-enrolled course - 409 NOT_ENROLLED", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/courses/%d/enroll", idTarget)
		resp, body, err := doRequest(env.app, "DELETE", url, studentToken, nil)
		if err != nil || resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d (body: %v)", resp.StatusCode, body)
		}
		if body["code"] != "NOT_ENROLLED" {
			t.Errorf("expected code NOT_ENROLLED, got %v", body["code"])
		}
	})

	// 6. POST /enrollments/swap with from == to -> 422 VALIDATION_ERROR
	t.Run("Swap same course - 422 VALIDATION_ERROR", func(t *testing.T) {
		swapReq := model.SwapRequest{
			FromCourseID: idSource,
			ToCourseID:   idSource,
		}
		resp, body, err := doRequest(env.app, "POST", "/api/v1/enrollments/swap", studentToken, swapReq)
		if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d (body: %v)", resp.StatusCode, body)
		}
		if body["code"] != "VALIDATION_ERROR" {
			t.Errorf("expected code VALIDATION_ERROR, got %v", body["code"])
		}
	})

	// 7. POST /enrollments/swap to valid course -> 200 OK
	t.Run("Swap to empty course - 200 OK", func(t *testing.T) {
		swapReq := model.SwapRequest{
			FromCourseID: idSource,
			ToCourseID:   idTarget,
		}
		resp, body, err := doRequest(env.app, "POST", "/api/v1/enrollments/swap", studentToken, swapReq)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %v)", resp.StatusCode, body)
		}
		if body["success"] != true {
			t.Errorf("expected success=true, got %v", body["success"])
		}

		// Verifikasi student sekarang terdaftar di idTarget dan bukan idSource
		mineResp, mineBody, _ := doRequest(env.app, "GET", "/api/v1/courses/mine", studentToken, nil)
		if mineResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", mineResp.StatusCode)
		}
		data := mineBody["data"].(map[string]any)
		coursesList := data["courses"].([]any)
		if len(coursesList) != 1 {
			t.Fatalf("expected 1 course, got %d", len(coursesList))
		}
		firstCourse := coursesList[0].(map[string]any)
		if firstCourse["code"] != "IF308" {
			t.Errorf("expected IF308, got %v", firstCourse["code"])
		}
	})

	// 8. Swap to full course -> 409 COURSE_FULL, student stays in original
	t.Run("Swap to full course - 409 COURSE_FULL and stay in origin", func(t *testing.T) {
		// Isi idFull sampai kuotanya penuh
		var fullQuota int
		_ = env.db.QueryRow(ctx, "SELECT quota FROM courses WHERE id = $1", idFull).Scan(&fullQuota)
		_, _ = env.db.Exec(ctx, "UPDATE courses SET quota = 1, enrolled_count = 1 WHERE id = $1", idFull)

		swapReq := model.SwapRequest{
			FromCourseID: idTarget,
			ToCourseID:   idFull,
		}
		resp, body, err := doRequest(env.app, "POST", "/api/v1/enrollments/swap", studentToken, swapReq)
		if err != nil || resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d (body: %v)", resp.StatusCode, body)
		}
		if body["code"] != "COURSE_FULL" {
			t.Errorf("expected code COURSE_FULL, got %v", body["code"])
		}

		// Kembalikan kuota normal idFull
		_, _ = env.db.Exec(ctx, "UPDATE courses SET quota = $1, enrolled_count = 0 WHERE id = $2", fullQuota, idFull)

		// Mahasiswa tetap di kelas asal
		var existsInOrigin bool
		_ = env.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM enrollments WHERE course_id = $1 AND user_id = $2)", idTarget, studentID).Scan(&existsInOrigin)
		if !existsInOrigin {
			t.Errorf("mahasiswa seharusnya tetap terdaftar di kelas asal (IF308)")
		}
	})

	// 9. PATCH /courses/:id - Student tries to update quota -> 403 Forbidden
	t.Run("Student updates quota - 403 Forbidden", func(t *testing.T) {
		patchReq := model.UpdateQuotaRequest{
			Quota:   50,
			Version: 1,
		}
		url := fmt.Sprintf("/api/v1/courses/%d", idSource)
		resp, body, err := doRequest(env.app, "PATCH", url, studentToken, patchReq)
		if err != nil || resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d (body: %v)", resp.StatusCode, body)
		}
		if body["code"] != "FORBIDDEN" {
			t.Errorf("expected code FORBIDDEN, got %v", body["code"])
		}
	})

	// 10. PATCH /courses/:id - Admin updates quota with latest version -> 200 OK
	t.Run("Admin updates quota with valid version - 200 OK", func(t *testing.T) {
		var currentVersion int
		_ = env.db.QueryRow(ctx, "SELECT version FROM courses WHERE id = $1", idSource).Scan(&currentVersion)

		patchReq := model.UpdateQuotaRequest{
			Quota:   50,
			Version: currentVersion,
		}
		url := fmt.Sprintf("/api/v1/courses/%d", idSource)
		resp, body, err := doRequest(env.app, "PATCH", url, adminToken, patchReq)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %v)", resp.StatusCode, body)
		}
		data := body["data"].(map[string]any)
		if data["version"].(float64) != float64(currentVersion+1) {
			t.Errorf("expected version %d, got %v", currentVersion+1, data["version"])
		}
	})

	// 11. PATCH /courses/:id - Admin updates quota with stale version -> 409 VERSION_CONFLICT
	t.Run("Admin updates quota with stale version - 409 VERSION_CONFLICT", func(t *testing.T) {
		patchReq := model.UpdateQuotaRequest{
			Quota:   60,
			Version: 1, // Stale version
		}
		url := fmt.Sprintf("/api/v1/courses/%d", idSource)
		resp, body, err := doRequest(env.app, "PATCH", url, adminToken, patchReq)
		if err != nil || resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d (body: %v)", resp.StatusCode, body)
		}
		if body["code"] != "VERSION_CONFLICT" {
			t.Errorf("expected code VERSION_CONFLICT, got %v", body["code"])
		}
	})

	// 12. DELETE /courses/:id/enroll - Drop enrolled course -> 204 NoContent
	t.Run("Drop enrolled course - 204 NoContent", func(t *testing.T) {
		url := fmt.Sprintf("/api/v1/courses/%d/enroll", idTarget)
		resp, _, err := doRequest(env.app, "DELETE", url, studentToken, nil)
		if err != nil || resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", resp.StatusCode)
		}
	})
}
