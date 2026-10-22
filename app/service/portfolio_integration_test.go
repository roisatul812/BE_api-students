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

type portfolioTestEnv struct {
	app        *fiber.App
	db         *pgxpool.Pool
	mongoDB    *mongo.Database
	jwtManager *helper.JWTManager
	admin      model.AuthUser
	ownerUser  model.AuthUser
	otherUser  model.AuthUser
	studentID  int
	otherStuID int
}

func setupPortfolioTestEnv(t *testing.T) *portfolioTestEnv {
	_ = godotenv.Load("../../.env")
	ctx := context.Background()

	db, err := database.Connect(ctx)
	if err != nil {
		t.Skipf("PostgreSQL tidak tersedia: %v", err)
	}

	mongoCfg := database.LoadMongoConfig()
	mongoClient, err := database.ConnectMongo(ctx, mongoCfg)
	if err != nil {
		t.Skipf("MongoDB tidak tersedia: %v", err)
	}

	testDBName := "praktikum_backend_test"
	mongoDB := database.GetMongoDatabase(mongoClient, testDBName)
	coll := mongoDB.Collection("student_portfolios")
	_ = coll.Drop(ctx)

	roleRepo := repository.NewRoleRepository(db)
	perms, err := roleRepo.LoadPermissions(ctx)
	if err != nil {
		t.Fatalf("load permissions error: %v", err)
	}
	permSet := helper.NewPermissionSet(perms)

	var adminID, ownerID, otherUserID int
	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, password, is_active, role)
		VALUES ('port_admin', 'port_admin@example.com', 'hash', true, 'admin')
		ON CONFLICT (username) DO UPDATE SET role = 'admin'
		RETURNING id
	`).Scan(&adminID)
	if err != nil {
		t.Fatalf("setup admin error: %v", err)
	}

	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, password, is_active, role)
		VALUES ('port_owner', 'port_owner@example.com', 'hash', true, 'user')
		ON CONFLICT (username) DO UPDATE SET role = 'user'
		RETURNING id
	`).Scan(&ownerID)
	if err != nil {
		t.Fatalf("setup owner user error: %v", err)
	}

	err = db.QueryRow(ctx, `
		INSERT INTO users (username, email, password, is_active, role)
		VALUES ('port_other', 'port_other@example.com', 'hash', true, 'user')
		ON CONFLICT (username) DO UPDATE SET role = 'user'
		RETURNING id
	`).Scan(&otherUserID)
	if err != nil {
		t.Fatalf("setup other user error: %v", err)
	}

	// Buat mahasiswa yang dimiliki oleh ownerID
	var studentID int
	err = db.QueryRow(ctx, `
		INSERT INTO students (nim, name, grade, is_active, owner_id)
		VALUES ('9988771', 'Student Portfolio Owner', 88, true, $1)
		ON CONFLICT (nim) DO UPDATE SET owner_id = EXCLUDED.owner_id
		RETURNING id
	`, ownerID).Scan(&studentID)
	if err != nil {
		t.Fatalf("setup student error: %v", err)
	}

	// Buat mahasiswa yang dimiliki oleh otherUserID
	var otherStuID int
	err = db.QueryRow(ctx, `
		INSERT INTO students (nim, name, grade, is_active, owner_id)
		VALUES ('9988772', 'Student Portfolio Other', 85, true, $1)
		ON CONFLICT (nim) DO UPDATE SET owner_id = EXCLUDED.owner_id
		RETURNING id
	`, otherUserID).Scan(&otherStuID)
	if err != nil {
		t.Fatalf("setup other student error: %v", err)
	}

	jwtManager := helper.NewJWTManager("test_secret_for_modul10_testing_32bytes!", "test-issuer", 1*time.Hour)

	portfolioRepo := repository.NewPortfolioRepository(mongoDB)
	if err := portfolioRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes error: %v", err)
	}
	studentRepo := repository.NewPostgresStudentRepository(db)
	portfolioService := service.NewPortfolioService(portfolioRepo, studentRepo, permSet)

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	app := fiber.New(fiber.Config{
		ErrorHandler: config.NewErrorHandler(logger),
	})

	route.PortfolioRoutes(app, portfolioService, jwtManager)

	t.Cleanup(func() {
		_ = coll.Drop(context.Background())
		_, _ = db.Exec(context.Background(), "DELETE FROM students WHERE id IN ($1, $2)", studentID, otherStuID)
		_, _ = db.Exec(context.Background(), "DELETE FROM users WHERE id IN ($1, $2, $3)", adminID, ownerID, otherUserID)
		_ = database.DisconnectMongo(mongoClient, 2*time.Second)
		db.Close()
	})

	return &portfolioTestEnv{
		app:        app,
		db:         db,
		mongoDB:    mongoDB,
		jwtManager: jwtManager,
		admin:      model.AuthUser{UserID: adminID, Username: "port_admin", Role: "admin"},
		ownerUser:  model.AuthUser{UserID: ownerID, Username: "port_owner", Role: "user"},
		otherUser:  model.AuthUser{UserID: otherUserID, Username: "port_other", Role: "user"},
		studentID:  studentID,
		otherStuID: otherStuID,
	}
}

func (e *portfolioTestEnv) token(user model.AuthUser) string {
	t, _ := e.jwtManager.GenerateAccess(model.User{
		ID:       user.UserID,
		Username: user.Username,
		Role:     user.Role,
	})
	return t
}

func (e *portfolioTestEnv) request(method, path string, token string, body interface{}) (*http.Response, map[string]interface{}) {
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

func TestPortfolioAPI_EndToEnd(t *testing.T) {
	env := setupPortfolioTestEnv(t)

	var createdProjectID string

	t.Run("GET /students/:id/portfolio without auth returns 401", func(t *testing.T) {
		resp, parsed := env.request("GET", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), "", nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("GET /students/:id/portfolio invalid student ID returns 400", func(t *testing.T) {
		token := env.token(env.ownerUser)
		resp, parsed := env.request("GET", "/api/v1/students/bukan-angka/portfolio", token, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("GET /students/:id/portfolio non-existent student in PostgreSQL returns 404", func(t *testing.T) {
		token := env.token(env.ownerUser)
		resp, parsed := env.request("GET", "/api/v1/students/999999/portfolio", token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("GET /students/:id/portfolio when portfolio not yet created returns 404", func(t *testing.T) {
		token := env.token(env.ownerUser)
		resp, parsed := env.request("GET", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("PUT /students/:id/portfolio by non-owner returns 403", func(t *testing.T) {
		token := env.token(env.otherUser) // bukan pemilik studentID
		payload := map[string]interface{}{
			"headline": "Unauthorized headline",
			"bio":      "Unauthorized biography long enough",
		}
		resp, parsed := env.request("PUT", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), token, payload)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("PUT /students/:id/portfolio with invalid payload returns 422", func(t *testing.T) {
		token := env.token(env.ownerUser)
		payload := map[string]interface{}{
			"headline": "Hi", // < 5 karakter
			"bio":      "Pendek",
		}
		resp, parsed := env.request("PUT", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), token, payload)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("PUT /students/:id/portfolio by owner creates portfolio successfully", func(t *testing.T) {
		token := env.token(env.ownerUser)
		payload := map[string]interface{}{
			"headline": "Golang Backend Developer",
			"bio":      "Mahasiswa D4 Teknik Informatika fokus pada arsitektur backend skala besar.",
			"projects": []map[string]interface{}{
				{
					"title":        "Sistem Microservices",
					"technologies": []string{"Golang", "PostgreSQL", "Docker"},
					"link":         "https://github.com/example/ms",
					"year":         2024,
				},
			},
			"certificates": []map[string]interface{}{
				{
					"name":   "Certified Kubernetes Administrator",
					"issuer": "CNCF",
					"date":   "2024-02-01",
				},
			},
		}
		resp, parsed := env.request("PUT", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), token, payload)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}

		data := parsed["data"].(map[string]interface{})
		if data["headline"] != "Golang Backend Developer" {
			t.Errorf("headline salah: %v", data["headline"])
		}

		projects := data["projects"].([]interface{})
		if len(projects) != 1 {
			t.Fatalf("proyek harus berjumlah 1, got %d", len(projects))
		}
		proj0 := projects[0].(map[string]interface{})
		createdProjectID = proj0["id"].(string)
		if createdProjectID == "" {
			t.Errorf("project ID tidak boleh kosong")
		}
	})

	t.Run("GET /students/:id/portfolio returns created portfolio", func(t *testing.T) {
		token := env.token(env.ownerUser)
		resp, parsed := env.request("GET", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), token, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}
		data := parsed["data"].(map[string]interface{})
		if data["headline"] != "Golang Backend Developer" {
			t.Errorf("headline salah: %v", data["headline"])
		}
	})

	t.Run("PUT /students/:id/portfolio by admin updates portfolio successfully", func(t *testing.T) {
		token := env.token(env.admin)
		payload := map[string]interface{}{
			"headline": "Updated by Admin",
			"bio":      "Biografi yang telah diperbarui oleh administrator kampus.",
			"projects": []map[string]interface{}{},
		}
		resp, parsed := env.request("PUT", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), token, payload)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}
		data := parsed["data"].(map[string]interface{})
		if data["headline"] != "Updated by Admin" {
			t.Errorf("headline gagal diupdate oleh admin: %v", data["headline"])
		}
	})

	t.Run("POST /students/:id/portfolio/projects by non-owner returns 403", func(t *testing.T) {
		token := env.token(env.otherUser)
		payload := map[string]interface{}{
			"title":        "Unauthorized Project",
			"technologies": []string{"c++"},
			"year":         2024,
		}
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/students/%d/portfolio/projects", env.studentID), token, payload)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("POST /students/:id/portfolio/projects by owner adds project successfully", func(t *testing.T) {
		token := env.token(env.ownerUser)
		payload := map[string]interface{}{
			"title":        "Payment Gateway Integration",
			"technologies": []string{"Go", "Redis", "Kafka"},
			"link":         "https://example.com/pg",
			"year":         2024,
		}
		resp, parsed := env.request("POST", fmt.Sprintf("/api/v1/students/%d/portfolio/projects", env.studentID), token, payload)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %v", resp.StatusCode, parsed)
		}
		data := parsed["data"].(map[string]interface{})
		projects := data["projects"].([]interface{})
		if len(projects) != 1 {
			t.Fatalf("proyek harus bertambah jadi 1, got %d", len(projects))
		}
		proj := projects[0].(map[string]interface{})
		createdProjectID = proj["id"].(string)
	})

	t.Run("DELETE /students/:id/portfolio/projects/:projectId by non-owner returns 403", func(t *testing.T) {
		token := env.token(env.otherUser)
		resp, parsed := env.request("DELETE", fmt.Sprintf("/api/v1/students/%d/portfolio/projects/%s", env.studentID, createdProjectID), token, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("DELETE /students/:id/portfolio/projects/:projectId with invalid projectId returns 400", func(t *testing.T) {
		token := env.token(env.ownerUser)
		resp, parsed := env.request("DELETE", fmt.Sprintf("/api/v1/students/%d/portfolio/projects/bukan-hex", env.studentID), token, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("DELETE /students/:id/portfolio/projects/:projectId with non-existent projectId returns 404", func(t *testing.T) {
		token := env.token(env.ownerUser)
		randomHex := bson.NewObjectID().Hex()
		resp, parsed := env.request("DELETE", fmt.Sprintf("/api/v1/students/%d/portfolio/projects/%s", env.studentID, randomHex), token, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %v", resp.StatusCode, parsed)
		}
	})

	t.Run("DELETE /students/:id/portfolio/projects/:projectId by owner returns 204", func(t *testing.T) {
		token := env.token(env.ownerUser)
		resp, _ := env.request("DELETE", fmt.Sprintf("/api/v1/students/%d/portfolio/projects/%s", env.studentID, createdProjectID), token, nil)
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", resp.StatusCode)
		}

		// Verifikasi proyek sudah terhapus
		respGet, parsedGet := env.request("GET", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), token, nil)
		if respGet.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", respGet.StatusCode)
		}
		data := parsedGet["data"].(map[string]interface{})
		projects := data["projects"].([]interface{})
		if len(projects) != 0 {
			t.Fatalf("proyek harusnya kosong setelah dihapus, got %d", len(projects))
		}
	})

	t.Run("GET /portfolios/stats/technologies returns 200 with aggregation results", func(t *testing.T) {
		// Tambahkan proyek baru agar ada data untuk agregasi
		tokenOwner := env.token(env.ownerUser)
		_, _ = env.request("POST", fmt.Sprintf("/api/v1/students/%d/portfolio/projects", env.studentID), tokenOwner, map[string]interface{}{
			"title":        "Project Go Redis",
			"technologies": []string{"Go", "Redis"},
			"year":         2024,
		})

		tokenOther := env.token(env.otherUser)
		// Buat portfolio untuk other student
		_, _ = env.request("PUT", fmt.Sprintf("/api/v1/students/%d/portfolio", env.otherStuID), tokenOther, map[string]interface{}{
			"headline": "AI Specialist",
			"bio":      "Mahasiswa D4 Teknik Informatika fokus pada machine learning dan NLP.",
			"projects": []map[string]interface{}{
				{
					"title":        "AI Sentiment Analysis",
					"technologies": []string{"Python", "Go"},
					"year":         2024,
				},
			},
		})

		resp, parsed := env.request("GET", "/api/v1/portfolios/stats/technologies", tokenOwner, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d: %v", resp.StatusCode, parsed)
		}

		data := parsed["data"].([]interface{})
		if len(data) == 0 {
			t.Fatalf("agregasi teknologi tidak boleh kosong")
		}

		top := data[0].(map[string]interface{})
		if top["technology"] != "go" || top["count"] != float64(2) {
			t.Errorf("teknologi teratas harus 'go' dengan count 2, got: %+v", top)
		}
	})

	t.Run("NoSQL injection attempt via body returns 400", func(t *testing.T) {
		token := env.token(env.ownerUser)
		raw := `{"headline": {"$ne": ""}, "bio": "Valid bio text that is long enough"}`
		resp, parsed := env.request("PUT", fmt.Sprintf("/api/v1/students/%d/portfolio", env.studentID), token, raw)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for NoSQL operator object in body, got %d: %v", resp.StatusCode, parsed)
		}
	})
}
