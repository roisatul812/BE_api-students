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
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type reviewTestEnv struct {
	app            *fiber.App
	db             *pgxpool.Pool
	mongoDB        *mongo.Database
	jwtManager     *helper.JWTManager
	admin          model.AuthUser
	mhsEnrolled    model.AuthUser
	mhsOther       model.AuthUser
	mhsNotEnrolled model.AuthUser
	courseID       int
}

func setupReviewTestEnv(t *testing.T) *reviewTestEnv {
	_ = godotenv.Load("../../.env")
	ctx := context.Background()

	// PostgreSQL
	db, err := database.Connect(ctx)
	if err != nil {
		t.Skipf("PostgreSQL tidak tersedia: %v", err)
	}

	// MongoDB
	mongoCfg := database.LoadMongoConfig()
	mongoClient, err := database.ConnectMongo(ctx, mongoCfg)
	if err != nil {
		t.Skipf("MongoDB tidak tersedia: %v", err)
	}

	testDBName := "praktikum_backend_test"
	mongoDB := database.GetMongoDatabase(mongoClient, testDBName)
	coll := mongoDB.Collection("reviews")
	_ = coll.Drop(ctx)

	// Persiapkan test users di PostgreSQL
	// Pastikan roles dan permissions sudah ada
	roleRepo := repository.NewRoleRepository(db)
	perms, err := roleRepo.LoadPermissions(ctx)
	if err != nil {
		t.Fatalf("load permissions error: %v", err)
	}
	permSet := helper.NewPermissionSet(perms)

	var adminID, mhs1ID, mhs2ID, mhs3ID int
	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, password, is_active, role)
		VALUES ('rev_admin', 'rev_admin@example.com', 'hash', true, 'admin')
		ON CONFLICT (username) DO UPDATE SET role = 'admin'
		RETURNING id
	`).Scan(&adminID)
	if err != nil {
		t.Fatalf("setup admin error: %v", err)
	}

	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, password, is_active, role)
		VALUES ('rev_mhs1', 'rev_mhs1@example.com', 'hash', true, 'user')
		ON CONFLICT (username) DO UPDATE SET role = 'user'
		RETURNING id
	`).Scan(&mhs1ID)
	if err != nil {
		t.Fatalf("setup mhs1 error: %v", err)
	}

	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, password, is_active, role)
		VALUES ('rev_mhs2', 'rev_mhs2@example.com', 'hash', true, 'user')
		ON CONFLICT (username) DO UPDATE SET role = 'user'
		RETURNING id
	`).Scan(&mhs2ID)
	if err != nil {
		t.Fatalf("setup mhs2 error: %v", err)
	}

	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, password, is_active, role)
		VALUES ('rev_mhs3', 'rev_mhs3@example.com', 'hash', true, 'user')
		ON CONFLICT (username) DO UPDATE SET role = 'user'
		RETURNING id
	`).Scan(&mhs3ID)
	if err != nil {
		t.Fatalf("setup mhs3 error: %v", err)
	}

	// Pastikan course 1 (IF101) ada di Postgres
	var courseID int
	err = db.QueryRow(ctx, `
		INSERT INTO courses (code, name, credits, quota, enrolled_count)
		VALUES ('IF101_TEST', 'Pemrograman Backend Lanjut Test', 3, 40, 1)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`).Scan(&courseID)
	if err != nil {
		t.Fatalf("setup course error: %v", err)
	}

	// Daftarkan mhs1 ke course ini, pastikan mhs3 TIDAK terdaftar
	_, _ = db.Exec(ctx, "DELETE FROM enrollments WHERE course_id = $1", courseID)
	_, err = db.Exec(ctx, "INSERT INTO enrollments (course_id, user_id) VALUES ($1, $2)", courseID, mhs1ID)
	if err != nil {
		t.Fatalf("setup enrollment error: %v", err)
	}

	jwtManager := helper.NewJWTManager("test_secret_for_modul10_testing_32bytes!", "test-issuer", 1*time.Hour)

	reviewRepo := repository.NewReviewRepository(mongoDB)
	if err := reviewRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes error: %v", err)
	}
	courseRepo := repository.NewCourseRepository(db)
	reviewService := service.NewReviewService(reviewRepo, courseRepo, permSet)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	app := fiber.New(fiber.Config{
		ErrorHandler: config.NewErrorHandler(logger),
	})

	route.ReviewRoutes(app, reviewService, jwtManager)

	t.Cleanup(func() {
		_ = coll.Drop(context.Background())
		_, _ = db.Exec(context.Background(), "DELETE FROM enrollments WHERE course_id = $1", courseID)
		_, _ = db.Exec(context.Background(), "DELETE FROM courses WHERE id = $1", courseID)
		_, _ = db.Exec(context.Background(), "DELETE FROM users WHERE id IN ($1, $2, $3, $4)", adminID, mhs1ID, mhs2ID, mhs3ID)
		_ = database.DisconnectMongo(mongoClient, 2*time.Second)
		db.Close()
	})

	return &reviewTestEnv{
		app:            app,
		db:             db,
		mongoDB:        mongoDB,
		jwtManager:     jwtManager,
		admin:          model.AuthUser{UserID: adminID, Username: "rev_admin", Role: "admin"},
		mhsEnrolled:    model.AuthUser{UserID: mhs1ID, Username: "rev_mhs1", Role: "user"},
		mhsOther:       model.AuthUser{UserID: mhs2ID, Username: "rev_mhs2", Role: "user"},
		mhsNotEnrolled: model.AuthUser{UserID: mhs3ID, Username: "rev_mhs3", Role: "user"},
		courseID:       courseID,
	}
}

func (e *reviewTestEnv) token(user model.AuthUser) string {
	t, _ := e.jwtManager.GenerateAccess(model.User{
		ID:       user.UserID,
		Username: user.Username,
		Role:     user.Role,
	})
	return t
}

func (e *reviewTestEnv) request(method, path string, token string, body interface{}) (*http.Response, map[string]interface{}) {
	var bodyReader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			bodyReader = bytes.NewBufferString(v)
		case []byte:
			bodyReader = bytes.NewBuffer(v)
		default:
			b, _ := json.Marshal(v)
			bodyReader = bytes.NewBuffer(b)
		}
	}

	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := e.app.Test(req, 10000)
	if err != nil {
		panic(err)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var parsed map[string]interface{}
	_ = json.Unmarshal(respBody, &parsed)

	return resp, parsed
}

func TestReviewAPI_EndToEnd(t *testing.T) {
	env := setupReviewTestEnv(t)

	var createdReviewID string

	t.Run("POST /courses/:id/reviews without auth returns 401", func(t *testing.T) {
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/courses/%d/reviews", env.courseID), "", map[string]interface{}{
			"rating":  5,
			"comment": "Ulasan tanpa autentikasi",
		})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("POST /courses/:id/reviews on non-existent course returns 404", func(t *testing.T) {
		token := env.token(env.mhsEnrolled)
		resp, parsed := env.request("POST", "/api/v1/courses/999999/reviews", token, map[string]interface{}{
			"rating":  5,
			"comment": "Ulasan untuk mata kuliah fiktif",
		})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("POST /courses/:id/reviews by student not enrolled returns 403 NOT_ENROLLED", func(t *testing.T) {
		token := env.token(env.mhsNotEnrolled)
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/courses/%d/reviews", env.courseID), token, map[string]interface{}{
			"rating":  5,
			"comment": "Saya bukan peserta kelas ini tetapi mencoba menulis ulasan",
		})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %v", resp.StatusCode, parsed)
		}
		if parsed["code"] != "NOT_ENROLLED" {
			t.Errorf("expected code NOT_ENROLLED, got: %v", parsed["code"])
		}
	})

	t.Run("POST /courses/:id/reviews with invalid rating returns 422", func(t *testing.T) {
		token := env.token(env.mhsEnrolled)
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/courses/%d/reviews", env.courseID), token, map[string]interface{}{
			"rating":  9,
			"comment": "Rating melebihi batas maksimal",
		})
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("POST /courses/:id/reviews with short comment returns 422", func(t *testing.T) {
		token := env.token(env.mhsEnrolled)
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/courses/%d/reviews", env.courseID), token, map[string]interface{}{
			"rating":  4,
			"comment": "pendek",
		})
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("POST /courses/:id/reviews success with Location header", func(t *testing.T) {
		token := env.token(env.mhsEnrolled)
		payload := map[string]interface{}{
			"rating":  5,
			"comment": "Materi Go sangat jelas, tugas menantang dan bermanfaat",
			"tags":    []string{"Tugas-Banyak", " dosen-jelas ", "tugas-banyak"},
		}
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/courses/%d/reviews", env.courseID), token, payload)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %v", resp.StatusCode, parsed)
		}

		loc := resp.Header.Get("Location")
		if loc == "" {
			t.Errorf("header Location wajib ada pada respons 201")
		}

		data := parsed["data"].(map[string]interface{})
		createdReviewID = data["id"].(string)

		tags := data["tags"].([]interface{})
		if len(tags) != 2 || tags[0] != "tugas-banyak" || tags[1] != "dosen-jelas" {
			t.Errorf("tags tidak ternormalisasi dengan benar: %v", tags)
		}
	})

	t.Run("POST /courses/:id/reviews duplicate review by same student returns 409 ALREADY_REVIEWED", func(t *testing.T) {
		token := env.token(env.mhsEnrolled)
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/courses/%d/reviews", env.courseID), token, map[string]interface{}{
			"rating":  4,
			"comment": "Ulasan kedua dari mahasiswa yang sama harus ditolak",
		})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %v", resp.StatusCode, parsed)
		}
		if parsed["code"] != "ALREADY_REVIEWED" {
			t.Errorf("expected code ALREADY_REVIEWED, got: %v", parsed["code"])
		}
	})

	t.Run("GET /courses/:id/reviews returns list with pagination meta", func(t *testing.T) {
		token := env.token(env.mhsOther)
		resp, parsed := env.request("GET", fmt.Sprintf("/api/v1/courses/%d/reviews?limit=5", env.courseID), token, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}

		data := parsed["data"].([]interface{})
		if len(data) != 1 {
			t.Errorf("expected 1 review, got %d", len(data))
		}

		meta := parsed["meta"].(map[string]interface{})
		if meta["limit"] != float64(5) || meta["has_more"] != false {
			t.Errorf("meta salah: %v", meta)
		}
	})

	t.Run("GET /courses/:id/reviews/summary returns valid aggregation stats", func(t *testing.T) {
		token := env.token(env.mhsOther)
		resp, parsed := env.request("GET", fmt.Sprintf("/api/v1/courses/%d/reviews/summary", env.courseID), token, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}

		data := parsed["data"].(map[string]interface{})
		if data["count"] != float64(1) || data["average_rating"] != float64(5) {
			t.Errorf("summary data salah: %v", data)
		}
	})

	t.Run("GET /reviews/:id returns 200", func(t *testing.T) {
		token := env.token(env.mhsOther)
		resp, parsed := env.request("GET", "/api/v1/reviews/"+createdReviewID, token, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}
		data := parsed["data"].(map[string]interface{})
		if data["id"] != createdReviewID {
			t.Errorf("expected review ID %s, got %v", createdReviewID, data["id"])
		}
	})

	t.Run("GET /reviews/bukan-objectid returns 400", func(t *testing.T) {
		token := env.token(env.mhsOther)
		resp, parsed := env.request("GET", "/api/v1/reviews/bukan-objectid", token, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("GET /reviews/:id not found returns 404", func(t *testing.T) {
		token := env.token(env.mhsOther)
		resp, parsed := env.request("GET", "/api/v1/reviews/"+bson.NewObjectID().Hex(), token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("PATCH /reviews/:id by non-owner returns 403", func(t *testing.T) {
		token := env.token(env.mhsOther)
		newRating := 3
		resp, parsed := env.request("PATCH", "/api/v1/reviews/"+createdReviewID, token, map[string]interface{}{
			"rating": newRating,
		})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("PATCH /reviews/:id with empty payload returns 400", func(t *testing.T) {
		token := env.token(env.mhsEnrolled)
		resp, parsed := env.request("PATCH", "/api/v1/reviews/"+createdReviewID, token, map[string]interface{}{})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("PATCH /reviews/:id by owner returns 200", func(t *testing.T) {
		token := env.token(env.mhsEnrolled)
		newRating := 4
		resp, parsed := env.request("PATCH", "/api/v1/reviews/"+createdReviewID, token, map[string]interface{}{
			"rating": newRating,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}
		data := parsed["data"].(map[string]interface{})
		if data["rating"] != float64(4) {
			t.Errorf("rating gagal diperbarui: %v", data)
		}
	})

	t.Run("POST /reviews/:id/helpful on own review returns 409 OWN_REVIEW", func(t *testing.T) {
		token := env.token(env.mhsEnrolled) // owner
		resp, parsed := env.request("POST", "/api/v1/reviews/"+createdReviewID+"/helpful", token, nil)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %v", resp.StatusCode, parsed)
		}
		if parsed["code"] != "OWN_REVIEW" {
			t.Errorf("expected code OWN_REVIEW, got: %v", parsed["code"])
		}
	})

	t.Run("POST /reviews/:id/helpful by another student returns 200", func(t *testing.T) {
		token := env.token(env.mhsOther)
		resp, parsed := env.request("POST", "/api/v1/reviews/"+createdReviewID+"/helpful", token, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}
		data := parsed["data"].(map[string]interface{})
		if data["helpful_count"] != float64(1) {
			t.Errorf("helpful_count salah: %v", data)
		}
	})

	t.Run("POST /reviews/:id/helpful duplicate vote returns 409 ALREADY_VOTED", func(t *testing.T) {
		token := env.token(env.mhsOther)
		resp, parsed := env.request("POST", "/api/v1/reviews/"+createdReviewID+"/helpful", token, nil)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409, got %d: %v", resp.StatusCode, parsed)
		}
		if parsed["code"] != "ALREADY_VOTED" {
			t.Errorf("expected code ALREADY_VOTED, got: %v", parsed["code"])
		}
	})

	t.Run("POST /reviews/:id/replies adds reply successfully", func(t *testing.T) {
		token := env.token(env.mhsNotEnrolled) // siapapun yang login boleh membalas
		resp, parsed := env.request("POST", "/api/v1/reviews/"+createdReviewID+"/replies", token, map[string]interface{}{
			"body": "Terima kasih atas ulasannya!",
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %v", resp.StatusCode, parsed)
		}
		data := parsed["data"].(map[string]interface{})
		replies := data["replies"].([]interface{})
		if len(replies) != 1 {
			t.Errorf("replies harus 1, got %d", len(replies))
		}
	})

	t.Run("DELETE /reviews/:id by non-owner returns 403", func(t *testing.T) {
		token := env.token(env.mhsOther)
		resp, parsed := env.request("DELETE", "/api/v1/reviews/"+createdReviewID, token, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("DELETE /reviews/:id by admin with review:moderate returns 204", func(t *testing.T) {
		token := env.token(env.admin)
		resp, _ := env.request("DELETE", "/api/v1/reviews/"+createdReviewID, token, nil)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", resp.StatusCode)
		}

		// Verifikasi sudah hilang
		resp2, _ := env.request("GET", "/api/v1/reviews/"+createdReviewID, token, nil)
		if resp2.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 after deletion, got %d", resp2.StatusCode)
		}
	})

	t.Run("DELETE /reviews/:id already deleted returns 404", func(t *testing.T) {
		token := env.token(env.admin)
		resp, _ := env.request("DELETE", "/api/v1/reviews/"+createdReviewID, token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("NoSQL injection attempt via body parser returns 400", func(t *testing.T) {
		token := env.token(env.mhsEnrolled)
		raw := `{"rating": 5, "comment": {"$ne": ""}}`
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/courses/%d/reviews", env.courseID), token, raw)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for NoSQL operator object in body, got %d: %v", resp.StatusCode, parsed)
		}
	})
}
