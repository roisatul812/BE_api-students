package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/database"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func setupTestMongoPortfolio(t *testing.T) (*mongo.Client, *mongo.Database, repository.PortfolioRepository) {
	_ = godotenv.Load("../../.env")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := database.LoadMongoConfig()
	client, err := database.ConnectMongo(ctx, cfg)
	if err != nil {
		t.Skipf("MongoDB tidak tersedia: %v", err)
	}

	testDBName := "praktikum_backend_test"
	db := database.GetMongoDatabase(client, testDBName)

	coll := db.Collection("student_portfolios")
	_ = coll.Drop(ctx)

	repo := repository.NewPortfolioRepository(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	t.Cleanup(func() {
		_ = coll.Drop(context.Background())
		_ = database.DisconnectMongo(client, 2*time.Second)
	})

	return client, db, repo
}

func TestPortfolioRepository_CRUD(t *testing.T) {
	_, db, repo := setupTestMongoPortfolio(t)
	ctx := context.Background()

	studentID := 101

	t.Run("Upsert creates new portfolio", func(t *testing.T) {
		p, err := repo.Upsert(ctx, model.Portfolio{
			StudentID: studentID,
			Headline:  "Full-Stack Developer",
			Bio:       "Suka membangun aplikasi web modern dan scalable.",
			Projects: []model.Project{
				{
					Title:        "Project 1",
					Technologies: []string{"go", "react"},
					Year:         2024,
				},
			},
			Certificates: []model.Certificate{
				{
					Name:   "Google Cloud Certified",
					Issuer: "Google",
					Date:   "2024-01-01",
				},
			},
		})
		if err != nil {
			t.Fatalf("Upsert gagal: %v", err)
		}
		if p.StudentID != studentID || p.Headline != "Full-Stack Developer" {
			t.Errorf("data portfolio salah: %+v", p)
		}
		if len(p.Projects) != 1 || p.Projects[0].ID == "" {
			t.Errorf("project ID harus digenerate: %+v", p.Projects)
		}
	})

	t.Run("FindByStudentID returns existing portfolio", func(t *testing.T) {
		p, err := repo.FindByStudentID(ctx, studentID)
		if err != nil {
			t.Fatalf("FindByStudentID gagal: %v", err)
		}
		if p.StudentID != studentID || len(p.Projects) != 1 {
			t.Errorf("hasil FindByStudentID tidak sesuai: %+v", p)
		}
	})

	t.Run("FindByStudentID not found returns ErrPortfolioNotFound", func(t *testing.T) {
		_, err := repo.FindByStudentID(ctx, 99999)
		if !errors.Is(err, repository.ErrPortfolioNotFound) {
			t.Fatalf("seharusnya ErrPortfolioNotFound, got: %v", err)
		}
	})

	t.Run("Unique index on student_id prevents duplicate portfolio documents", func(t *testing.T) {
		coll := db.Collection("student_portfolios")
		_, err := coll.InsertOne(ctx, bson.M{
			"student_id": studentID,
			"headline":   "Duplicate headline",
		})
		if err == nil {
			t.Fatalf("seharusnya gagal karena duplicate key pada student_id")
		}
		if !mongo.IsDuplicateKeyError(err) {
			t.Fatalf("seharusnya duplicate key error, got: %v", err)
		}
	})

	t.Run("Upsert updates existing headline and bio", func(t *testing.T) {
		updated, err := repo.Upsert(ctx, model.Portfolio{
			StudentID: studentID,
			Headline:  "Senior Cloud Architect",
			Bio:       "Biografi yang diperbarui dengan pengalaman arsitektur mikroservis.",
			Projects:  []model.Project{},
		})
		if err != nil {
			t.Fatalf("Upsert update gagal: %v", err)
		}
		if updated.Headline != "Senior Cloud Architect" {
			t.Errorf("headline gagal diperbarui: %v", updated.Headline)
		}
	})
}

func TestPortfolioRepository_AddProjectAnd20LimitConcurrency(t *testing.T) {
	_, _, repo := setupTestMongoPortfolio(t)
	ctx := context.Background()

	studentID := 202

	// Buat portfolio awal
	_, err := repo.Upsert(ctx, model.Portfolio{
		StudentID: studentID,
		Headline:  "Mahasiswa Berprestasi",
		Bio:       "Spesialisasi cloud backend dan concurrency.",
	})
	if err != nil {
		t.Fatalf("setup initial portfolio gagal: %v", err)
	}

	// Isi awal sampai 18 proyek
	for i := 1; i <= 18; i++ {
		_, err := repo.AddProject(ctx, studentID, model.Project{
			Title:        fmt.Sprintf("Project %d", i),
			Technologies: []string{"go", "postgresql"},
			Year:         2024,
		})
		if err != nil {
			t.Fatalf("tambah project %d gagal: %v", i, err)
		}
	}

	// Verifikasi sudah ada 18 proyek
	p, err := repo.FindByStudentID(ctx, studentID)
	if err != nil || len(p.Projects) != 18 {
		t.Fatalf("portfolio harus memiliki 18 proyek, got: %d, err: %v", len(p.Projects), err)
	}

	// UJI CONCURRENCY WAJIB:
	// Kirim 10 request penambahan proyek secara bersamaan.
	// Karena kuota tersisa 2 (19 dan 20), tepat 2 request harus berhasil, dan 8 request harus ditolak dengan ErrProjectLimit!
	concurrency := 10
	var wg sync.WaitGroup
	var successCount int64
	var limitCount int64
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := repo.AddProject(ctx, studentID, model.Project{
				Title:        fmt.Sprintf("Concurrent Project %d", idx),
				Technologies: []string{"docker", "redis"},
				Year:         2024,
			})

			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else if errors.Is(err, repository.ErrProjectLimit) {
				limitCount++
			}
		}(i)
	}
	wg.Wait()

	if successCount != 2 {
		t.Errorf("seharusnya tepat 2 request yang berhasil, got: %d", successCount)
	}
	if limitCount != 8 {
		t.Errorf("seharusnya tepat 8 request yang ditolak ErrProjectLimit, got: %d", limitCount)
	}

	// Verifikasi jumlah akhir tepat 20
	finalP, err := repo.FindByStudentID(ctx, studentID)
	if err != nil {
		t.Fatalf("FindByStudentID gagal: %v", err)
	}
	if len(finalP.Projects) != 20 {
		t.Fatalf("jumlah proyek akhir harus tepat 20, got: %d", len(finalP.Projects))
	}

	// Percobaan proyek ke-21 harus langsung ditolak dengan ErrProjectLimit
	_, err = repo.AddProject(ctx, studentID, model.Project{
		Title:        "Project 21",
		Technologies: []string{"rust"},
		Year:         2024,
	})
	if !errors.Is(err, repository.ErrProjectLimit) {
		t.Fatalf("proyek ke-21 harus menghasilkan ErrProjectLimit, got: %v", err)
	}
}

func TestPortfolioRepository_DeleteProject(t *testing.T) {
	_, _, repo := setupTestMongoPortfolio(t)
	ctx := context.Background()

	studentID := 303

	// Buat portfolio dengan 2 project
	p, err := repo.Upsert(ctx, model.Portfolio{
		StudentID: studentID,
		Headline:  "Mobile Developer",
		Bio:       "Flutter and native Android developer.",
		Projects: []model.Project{
			{Title: "App A", Technologies: []string{"flutter"}, Year: 2023},
			{Title: "App B", Technologies: []string{"kotlin"}, Year: 2024},
		},
	})
	if err != nil {
		t.Fatalf("Upsert gagal: %v", err)
	}

	projectToDelete := p.Projects[0].ID
	remainingProject := p.Projects[1].ID

	t.Run("Delete existing project succeeds", func(t *testing.T) {
		err := repo.DeleteProject(ctx, studentID, projectToDelete)
		if err != nil {
			t.Fatalf("DeleteProject gagal: %v", err)
		}

		checkP, err := repo.FindByStudentID(ctx, studentID)
		if err != nil {
			t.Fatalf("FindByStudentID gagal: %v", err)
		}
		if len(checkP.Projects) != 1 {
			t.Fatalf("jumlah proyek harus tersisa 1, got %d", len(checkP.Projects))
		}
		if checkP.Projects[0].ID != remainingProject {
			t.Errorf("proyek yang tersisa harus App B, got: %+v", checkP.Projects[0])
		}
	})

	t.Run("Delete non-existent project returns ErrProjectNotFound", func(t *testing.T) {
		randomHex := bson.NewObjectID().Hex()
		err := repo.DeleteProject(ctx, studentID, randomHex)
		if !errors.Is(err, repository.ErrProjectNotFound) {
			t.Fatalf("seharusnya ErrProjectNotFound, got: %v", err)
		}
	})

	t.Run("Delete with invalid hex returns ErrInvalidID", func(t *testing.T) {
		err := repo.DeleteProject(ctx, studentID, "bukan-hex")
		if !errors.Is(err, repository.ErrInvalidID) {
			t.Fatalf("seharusnya ErrInvalidID, got: %v", err)
		}
	})

	t.Run("Delete project on non-existent portfolio returns ErrPortfolioNotFound", func(t *testing.T) {
		randomHex := bson.NewObjectID().Hex()
		err := repo.DeleteProject(ctx, 99999, randomHex)
		if !errors.Is(err, repository.ErrPortfolioNotFound) {
			t.Fatalf("seharusnya ErrPortfolioNotFound, got: %v", err)
		}
	})
}

func TestPortfolioRepository_GetTechnologyStats(t *testing.T) {
	_, _, repo := setupTestMongoPortfolio(t)
	ctx := context.Background()

	t.Run("empty collection returns empty slice", func(t *testing.T) {
		stats, err := repo.GetTechnologyStats(ctx, 10)
		if err != nil {
			t.Fatalf("GetTechnologyStats gagal: %v", err)
		}
		if len(stats) != 0 {
			t.Errorf("ekspektasi slice kosong, got: %v", stats)
		}
	})

	t.Run("multiple portfolios aggregation computes top 10 accurately", func(t *testing.T) {
		// Mahasiswa 1: Go (2x), Docker (1x), Redis (1x)
		_, _ = repo.Upsert(ctx, model.Portfolio{
			StudentID: 401,
			Headline:  "Backend",
			Bio:       "Backend engineer bio test",
			Projects: []model.Project{
				{Title: "P1", Technologies: []string{"go", "docker"}, Year: 2024},
				{Title: "P2", Technologies: []string{"go", "redis"}, Year: 2024},
			},
		})

		// Mahasiswa 2: Go (1x), Python (2x), Docker (1x)
		_, _ = repo.Upsert(ctx, model.Portfolio{
			StudentID: 402,
			Headline:  "AI & Backend",
			Bio:       "AI and backend bio test",
			Projects: []model.Project{
				{Title: "P3", Technologies: []string{"python", "docker"}, Year: 2024},
				{Title: "P4", Technologies: []string{"python", "go"}, Year: 2024},
			},
		})

		// Mahasiswa 3: React (3x), TypeScript (2x), Docker (1x)
		_, _ = repo.Upsert(ctx, model.Portfolio{
			StudentID: 403,
			Headline:  "Frontend",
			Bio:       "Frontend bio test",
			Projects: []model.Project{
				{Title: "P5", Technologies: []string{"react", "typescript"}, Year: 2024},
				{Title: "P6", Technologies: []string{"react", "typescript", "docker"}, Year: 2024},
				{Title: "P7", Technologies: []string{"react"}, Year: 2024},
			},
		})

		// Hitungan ekspektasi:
		// docker: 4 (P1, P3, P6, P... wait: P1:1, P3:1, P6:1 = 3)
		// go: 3 (P1, P2, P4)
		// react: 3 (P5, P6, P7)
		// docker: 3 (P1, P3, P6)
		// python: 2 (P3, P4)
		// typescript: 2 (P5, P6)
		// redis: 1 (P2)
		stats, err := repo.GetTechnologyStats(ctx, 10)
		if err != nil {
			t.Fatalf("GetTechnologyStats gagal: %v", err)
		}
		if len(stats) < 6 {
			t.Fatalf("harus ada minimal 6 teknologi, got %d: %v", len(stats), stats)
		}

		// Pastikan teknologi teratas memiliki count 3
		if stats[0].Count != 3 {
			t.Errorf("teknologi teratas harus count 3, got %d (%s)", stats[0].Count, stats[0].Technology)
		}
	})
}
