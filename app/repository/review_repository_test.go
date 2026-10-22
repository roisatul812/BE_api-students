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

func setupTestMongoDB(t *testing.T) (*mongo.Client, *mongo.Database, repository.ReviewRepository) {
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

	// Bersihkan collection sebelum pengujian
	coll := db.Collection("reviews")
	_ = coll.Drop(ctx)

	repo := repository.NewReviewRepository(db)
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	t.Cleanup(func() {
		_ = coll.Drop(context.Background())
		_ = database.DisconnectMongo(client, 2*time.Second)
	})

	return client, db, repo
}

func TestReviewRepository_CRUD(t *testing.T) {
	_, _, repo := setupTestMongoDB(t)
	ctx := context.Background()

	var createdID string

	t.Run("Create review successfully", func(t *testing.T) {
		rev, err := repo.Create(ctx, model.Review{
			CourseID: 101,
			UserID:   1,
			Username: "mhs001",
			Rating:   5,
			Comment:  "Materi sangat bagus dan mendalam",
			Tags:     []string{"dosen-jelas", "praktikal"},
		})
		if err != nil {
			t.Fatalf("gagal membuat ulasan: %v", err)
		}
		if rev.ID == "" {
			t.Fatalf("ID ulasan kosong")
		}
		createdID = rev.ID
		if rev.CourseID != 101 || rev.Rating != 5 || len(rev.Tags) != 2 {
			t.Errorf("data ulasan tidak sesuai: %+v", rev)
		}
	})

	t.Run("Create duplicate review by same user on same course fails", func(t *testing.T) {
		_, err := repo.Create(ctx, model.Review{
			CourseID: 101,
			UserID:   1,
			Username: "mhs001",
			Rating:   4,
			Comment:  "Ulasan kedua yang seharusnya ditolak",
		})
		if !errors.Is(err, repository.ErrDuplicate) {
			t.Fatalf("seharusnya ErrDuplicate, got: %v", err)
		}
	})

	t.Run("FindByID valid", func(t *testing.T) {
		rev, err := repo.FindByID(ctx, createdID)
		if err != nil {
			t.Fatalf("FindByID gagal: %v", err)
		}
		if rev.ID != createdID || rev.Username != "mhs001" {
			t.Errorf("hasil FindByID salah: %+v", rev)
		}
	})

	t.Run("FindByID invalid format", func(t *testing.T) {
		_, err := repo.FindByID(ctx, "invalid-hex-id")
		if !errors.Is(err, repository.ErrInvalidID) {
			t.Fatalf("seharusnya ErrInvalidID, got: %v", err)
		}
	})

	t.Run("FindByID not found", func(t *testing.T) {
		dummyHex := bson.NewObjectID().Hex()
		_, err := repo.FindByID(ctx, dummyHex)
		if !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("seharusnya ErrNotFound, got: %v", err)
		}
	})

	t.Run("FindByCourse with tags and pagination", func(t *testing.T) {
		// Buat ulasan kedua oleh mahasiswa berbeda
		_, err := repo.Create(ctx, model.Review{
			CourseID: 101,
			UserID:   2,
			Username: "mhs002",
			Rating:   3,
			Comment:  "Tugas lumayan banyak",
			Tags:     []string{"tugas-banyak"},
		})
		if err != nil {
			t.Fatalf("gagal membuat ulasan kedua: %v", err)
		}

		// Cari semua ulasan course 101
		list, err := repo.FindByCourse(ctx, model.ReviewQuery{CourseID: 101, Limit: 10})
		if err != nil {
			t.Fatalf("FindByCourse gagal: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("ekspektasi 2 ulasan, dapat %d", len(list))
		}
		// Terbaru lebih dulu
		if list[0].UserID != 2 {
			t.Errorf("seharusnya ulasan mhs002 pertama karena terbaru")
		}

		// Filter berdasarkan tag
		listTag, err := repo.FindByCourse(ctx, model.ReviewQuery{CourseID: 101, Tag: "dosen-jelas", Limit: 10})
		if err != nil {
			t.Fatalf("filter tag gagal: %v", err)
		}
		if len(listTag) != 1 || listTag[0].UserID != 1 {
			t.Errorf("filter tag salah: %+v", listTag)
		}

		// Pagination dengan Before
		listPage, err := repo.FindByCourse(ctx, model.ReviewQuery{CourseID: 101, Before: list[0].ID, Limit: 10})
		if err != nil {
			t.Fatalf("pagination before gagal: %v", err)
		}
		if len(listPage) != 1 || listPage[0].ID != list[1].ID {
			t.Errorf("seharusnya hanya ulasan mhs001 di halaman kedua")
		}
	})

	t.Run("UpdateOwn by owner", func(t *testing.T) {
		newRating := 4
		newComment := "Komentar setelah direvisi menjadi lebih objektif"
		updated, err := repo.UpdateOwn(ctx, createdID, 1, model.UpdateReviewRequest{
			Rating:  &newRating,
			Comment: &newComment,
		})
		if err != nil {
			t.Fatalf("UpdateOwn gagal: %v", err)
		}
		if updated.Rating != 4 || updated.Comment != newComment {
			t.Errorf("hasil update salah: %+v", updated)
		}
	})

	t.Run("UpdateOwn by non-owner fails with ErrNotOwner", func(t *testing.T) {
		newRating := 1
		_, err := repo.UpdateOwn(ctx, createdID, 999, model.UpdateReviewRequest{
			Rating: &newRating,
		})
		if !errors.Is(err, repository.ErrNotOwner) {
			t.Fatalf("seharusnya ErrNotOwner, got: %v", err)
		}
	})

	t.Run("Delete by non-owner fails with ErrNotOwner", func(t *testing.T) {
		err := repo.Delete(ctx, createdID, 999, false)
		if !errors.Is(err, repository.ErrNotOwner) {
			t.Fatalf("seharusnya ErrNotOwner, got: %v", err)
		}
	})

	t.Run("Delete by admin (anyOwner=true)", func(t *testing.T) {
		err := repo.Delete(ctx, createdID, 999, true)
		if err != nil {
			t.Fatalf("Delete admin gagal: %v", err)
		}
		// Pastikan sudah terhapus
		_, err = repo.FindByID(ctx, createdID)
		if !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("seharusnya ulasan tidak ditemukan setelah dihapus")
		}
	})
}

func TestReviewRepository_AddReplyAndLimit(t *testing.T) {
	_, _, repo := setupTestMongoDB(t)
	ctx := context.Background()

	rev, err := repo.Create(ctx, model.Review{
		CourseID: 201,
		UserID:   10,
		Username: "mhs010",
		Rating:   5,
		Comment:  "Ulasan untuk pengujian batas balasan",
	})
	if err != nil {
		t.Fatalf("Create gagal: %v", err)
	}

	// Tambah balasan pertama
	reply1, err := repo.AddReply(ctx, rev.ID, model.Reply{
		UserID:   11,
		Username: "mhs011",
		Body:     "Setuju sekali!",
	})
	if err != nil {
		t.Fatalf("AddReply pertama gagal: %v", err)
	}
	if len(reply1.Replies) != 1 {
		t.Fatalf("jumlah replies harus 1, got %d", len(reply1.Replies))
	}

	// Isi sampai 48 balasan total
	for i := len(reply1.Replies); i < 48; i++ {
		_, err := repo.AddReply(ctx, rev.ID, model.Reply{
			UserID:   100 + i,
			Username: fmt.Sprintf("user%d", i),
			Body:     fmt.Sprintf("Balasan ke-%d", i+1),
		})
		if err != nil {
			t.Fatalf("isi balasan ke-%d gagal: %v", i+1, err)
		}
	}

	// Sekarang ada 48 balasan.
	// Jalankan uji batas 50 dan concurrency race:
	// Kirim 10 request bersamaan untuk menambah balasan!
	// Karena hanya tersisa 2 slot (49 dan 50), tepat 2 harus berhasil, dan 8 harus gagal dengan ErrReplyLimit!
	concurrency := 10
	var wg sync.WaitGroup
	var successCount int64
	var limitCount int64
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := repo.AddReply(ctx, rev.ID, model.Reply{
				UserID:   200 + idx,
				Username: fmt.Sprintf("concurrent_user_%d", idx),
				Body:     fmt.Sprintf("Concurrent reply %d", idx),
			})

			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successCount++
			} else if errors.Is(err, repository.ErrReplyLimit) {
				limitCount++
			}
		}(i)
	}
	wg.Wait()

	if successCount != 2 {
		t.Errorf("seharusnya tepat 2 request yang berhasil masuk, got %d", successCount)
	}
	if limitCount != 8 {
		t.Errorf("seharusnya 8 request ditolak dengan ErrReplyLimit, got %d", limitCount)
	}

	finalRev, err := repo.FindByID(ctx, rev.ID)
	if err != nil {
		t.Fatalf("FindByID gagal: %v", err)
	}
	if len(finalRev.Replies) != 50 {
		t.Fatalf("jumlah balasan akhir harus tepat 50, got %d", len(finalRev.Replies))
	}

	// Balasan ke-51 setelah penuh harus langsung ditolak ErrReplyLimit
	_, err = repo.AddReply(ctx, rev.ID, model.Reply{
		UserID:   999,
		Username: "late_user",
		Body:     "Balasan terlambat",
	})
	if !errors.Is(err, repository.ErrReplyLimit) {
		t.Fatalf("balasan ke-51 harus ErrReplyLimit, got: %v", err)
	}
}

func TestReviewRepository_MarkHelpful(t *testing.T) {
	_, _, repo := setupTestMongoDB(t)
	ctx := context.Background()

	rev, err := repo.Create(ctx, model.Review{
		CourseID: 301,
		UserID:   50,
		Username: "owner_mhs",
		Rating:   4,
		Comment:  "Ulasan untuk pengujian fitur helpful",
	})
	if err != nil {
		t.Fatalf("Create gagal: %v", err)
	}

	t.Run("owner cannot mark own review helpful", func(t *testing.T) {
		_, err := repo.MarkHelpful(ctx, rev.ID, 50)
		if !errors.Is(err, repository.ErrOwnReview) {
			t.Fatalf("seharusnya ErrOwnReview, got: %v", err)
		}
	})

	t.Run("other user can mark helpful", func(t *testing.T) {
		voted, err := repo.MarkHelpful(ctx, rev.ID, 51)
		if err != nil {
			t.Fatalf("MarkHelpful gagal: %v", err)
		}
		if voted.HelpfulCount != 1 {
			t.Errorf("helpful_count harus 1, got %d", voted.HelpfulCount)
		}
	})

	t.Run("same user marking again returns ErrAlreadyVoted", func(t *testing.T) {
		_, err := repo.MarkHelpful(ctx, rev.ID, 51)
		if !errors.Is(err, repository.ErrAlreadyVoted) {
			t.Fatalf("seharusnya ErrAlreadyVoted, got: %v", err)
		}
	})

	t.Run("concurrent votes from same user: exactly 1 succeeds, rest ErrAlreadyVoted", func(t *testing.T) {
		rev2, err := repo.Create(ctx, model.Review{
			CourseID: 302,
			UserID:   60,
			Username: "owner2",
			Rating:   5,
			Comment:  "Ulasan untuk race condition duplicate vote",
		})
		if err != nil {
			t.Fatalf("Create gagal: %v", err)
		}

		concurrency := 10
		var wg sync.WaitGroup
		var successCount int64
		var alreadyVotedCount int64
		var mu sync.Mutex

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := repo.MarkHelpful(ctx, rev2.ID, 61) // same user 61
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else if errors.Is(err, repository.ErrAlreadyVoted) {
					alreadyVotedCount++
				}
			}()
		}
		wg.Wait()

		if successCount != 1 {
			t.Errorf("seharusnya tepat 1 yang berhasil, got %d", successCount)
		}
		if alreadyVotedCount != 9 {
			t.Errorf("seharusnya 9 yang ditolak ErrAlreadyVoted, got %d", alreadyVotedCount)
		}

		checkRev, _ := repo.FindByID(ctx, rev2.ID)
		if checkRev.HelpfulCount != 1 {
			t.Errorf("helpful_count harus tetap 1, got %d", checkRev.HelpfulCount)
		}
	})

	t.Run("concurrent votes from 5 different users", func(t *testing.T) {
		rev3, _ := repo.Create(ctx, model.Review{
			CourseID: 303,
			UserID:   70,
			Username: "owner3",
			Rating:   4,
			Comment:  "Ulasan untuk 5 concurrent different users",
		})

		var wg sync.WaitGroup
		for u := 71; u <= 75; u++ {
			wg.Add(1)
			go func(userID int) {
				defer wg.Done()
				_, _ = repo.MarkHelpful(ctx, rev3.ID, userID)
			}(u)
		}
		wg.Wait()

		checkRev, _ := repo.FindByID(ctx, rev3.ID)
		if checkRev.HelpfulCount != 5 {
			t.Errorf("helpful_count harus 5, got %d", checkRev.HelpfulCount)
		}
	})
}

func TestReviewRepository_Summary(t *testing.T) {
	_, _, repo := setupTestMongoDB(t)
	ctx := context.Background()

	t.Run("empty course returns zeroed summary", func(t *testing.T) {
		sum, err := repo.Summary(ctx, 99999)
		if err != nil {
			t.Fatalf("Summary gagal: %v", err)
		}
		if sum.CourseID != 99999 || sum.Count != 0 || sum.AverageRating != 0 {
			t.Errorf("summary kosong salah: %+v", sum)
		}
		if sum.Distribution["1"] != 0 || sum.Distribution["5"] != 0 {
			t.Errorf("distribusi harus nol semua: %+v", sum.Distribution)
		}
		if len(sum.TopTags) != 0 {
			t.Errorf("top tags harus kosong: %+v", sum.TopTags)
		}
	})

	t.Run("course with 8 reviews matches Modul 10 example", func(t *testing.T) {
		courseID := 500
		// Ratings: 5, 5, 3, 4, 5, 2, 4, 5 (total 33 / 8 = 4.125 -> dibulatkan 4.13)
		ratings := []int{5, 5, 3, 4, 5, 2, 4, 5}
		tagsList := [][]string{
			{"dosen-jelas", "praktikal"},
			{"tugas-banyak", "dosen-jelas"},
			{"tugas-banyak"},
			{"praktikal", "seru"},
			{"dosen-jelas", "seru"},
			{"deadline-ketat"},
			{"praktikal", "tugas-banyak"},
			{"dosen-jelas"},
		}

		for i, rating := range ratings {
			_, err := repo.Create(ctx, model.Review{
				CourseID: courseID,
				UserID:   1000 + i,
				Username: fmt.Sprintf("user_%d", i),
				Rating:   rating,
				Comment:  fmt.Sprintf("Komentar mahasiswa ke-%d untuk kelas", i),
				Tags:     tagsList[i],
			})
			if err != nil {
				t.Fatalf("gagal membuat review ke-%d: %v", i, err)
			}
		}

		sum, err := repo.Summary(ctx, courseID)
		if err != nil {
			t.Fatalf("Summary gagal: %v", err)
		}
		if sum.Count != 8 {
			t.Errorf("count salah: got %d, expected 8", sum.Count)
		}
		if sum.AverageRating != 4.13 {
			t.Errorf("average_rating salah: got %f, expected 4.13", sum.AverageRating)
		}
		// Sebaran bintang: "1": 0, "2": 1, "3": 1, "4": 2, "5": 4
		if sum.Distribution["1"] != 0 || sum.Distribution["2"] != 1 || sum.Distribution["3"] != 1 ||
			sum.Distribution["4"] != 2 || sum.Distribution["5"] != 4 {
			t.Errorf("distribution salah: %+v", sum.Distribution)
		}

		// top_tags: dosen-jelas: 4, praktikal: 3, tugas-banyak: 3, seru: 2, deadline-ketat: 1
		if len(sum.TopTags) != 5 {
			t.Fatalf("top tags harus berjumlah 5, got %d", len(sum.TopTags))
		}
		if sum.TopTags[0].Tag != "dosen-jelas" || sum.TopTags[0].Total != 4 {
			t.Errorf("tag terpopuler 1 salah: %+v", sum.TopTags[0])
		}
	})
}
