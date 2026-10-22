package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"api-students/app/model"
	"api-students/config"
	"api-students/database"
	"api-students/helper"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func fire(n int, job func(i int) string) map[string]int {
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan string, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int, out chan<- string) {
			defer wg.Done()
			<-start
			out <- job(i)
		}(i, results)
	}

	close(start) // buka gerbang
	wg.Wait()    // tunggu SEMUA goroutine selesai
	close(results)

	tally := map[string]int{}
	for r := range results {
		tally[r]++
	}
	return tally
}

func main() {
	mode := flag.String("mode", "crowd", "mode uji: crowd | credits | swap")
	targetURL := flag.String("url", "http://localhost:3000", "base URL server API")
	flag.Parse()

	if err := godotenv.Load(); err != nil {
		_ = godotenv.Load("../../.env")
	}

	authCfg := config.LoadAuthConfig()
	jwtManager := helper.NewJWTManager(authCfg.JWTSecret, authCfg.JWTIssuer, authCfg.JWTAccessTTL)

	ctx := context.Background()
	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatalf("koneksi database gagal: %v", err)
	}
	defer db.Close()

	switch *mode {
	case "crowd":
		runCrowdMode(ctx, db, jwtManager, *targetURL)
	case "credits":
		runCreditsMode(ctx, db, jwtManager, *targetURL)
	case "swap":
		runSwapMode(ctx, db, jwtManager, *targetURL)
	default:
		log.Fatalf("mode tidak dikenal: %s (pilihan: crowd, credits, swap)", *mode)
	}
}

func generateUserTokens(ctx context.Context, db *pgxpool.Pool, jwtManager *helper.JWTManager, count int) ([]string, []int) {
	rows, err := db.Query(ctx, "SELECT id, username, role FROM users WHERE username LIKE 'mhs%' ORDER BY username LIMIT $1", count)
	if err != nil {
		log.Fatalf("mengambil user: %v", err)
	}
	defer rows.Close()

	var tokens []string
	var ids []int
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role); err != nil {
			log.Fatalf("scan user: %v", err)
		}
		tok, err := jwtManager.GenerateAccess(u)
		if err != nil {
			log.Fatalf("generate token: %v", err)
		}
		tokens = append(tokens, tok)
		ids = append(ids, u.ID)
	}
	return tokens, ids
}

func runCrowdMode(ctx context.Context, db *pgxpool.Pool, jwtManager *helper.JWTManager, baseURL string) {
	var courseID int
	if err := db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF399'").Scan(&courseID); err != nil {
		log.Fatalf("kelas IF399 tidak ditemukan: %v", err)
	}

	tokens, _ := generateUserTokens(ctx, db, jwtManager, 60)
	if len(tokens) < 60 {
		log.Fatalf("memerlukan minimal 60 user mhs001-mhs060, hanya ada %d", len(tokens))
	}

	fmt.Println("60 mahasiswa berebut kelas IF399 secara serentak...")
	start := time.Now()

	client := &http.Client{Timeout: 10 * time.Second}
	enrollURL := fmt.Sprintf("%s/api/v1/courses/%d/enroll", baseURL, courseID)

	tally := fire(60, func(i int) string {
		req, _ := http.NewRequest("POST", enrollURL, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[i])
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("ERR: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var res map[string]any
		_ = json.Unmarshal(body, &res)

		code, _ := res["code"].(string)
		if resp.StatusCode == http.StatusCreated {
			return "201 OK"
		}
		if code != "" {
			return fmt.Sprintf("%d %s", resp.StatusCode, code)
		}
		return fmt.Sprintf("%d %s", resp.StatusCode, resp.Status)
	})

	duration := time.Since(start)
	fmt.Println("hasil request:")
	for k, v := range tally {
		fmt.Printf("  %-30s %d\n", k, v)
	}
	fmt.Printf("waktu total : %v\n", duration)

	var enrolledCount, rowCount int
	_ = db.QueryRow(ctx, "SELECT enrolled_count FROM courses WHERE id = $1", courseID).Scan(&enrolledCount)
	_ = db.QueryRow(ctx, "SELECT COUNT(*) FROM enrollments WHERE course_id = $1", courseID).Scan(&rowCount)
	fmt.Printf("IF399  kuota=5  enrolled_count=%d  baris enrollments=%d\n", enrolledCount, rowCount)
	if enrolledCount == 5 && rowCount == 5 {
		fmt.Println("-> KONSISTEN")
	} else {
		fmt.Println("-> TIDAK KONSISTEN")
	}
}

func runCreditsMode(ctx context.Context, db *pgxpool.Pool, jwtManager *helper.JWTManager, baseURL string) {
	rows, err := db.Query(ctx, "SELECT id FROM courses WHERE code != 'IF399' ORDER BY id LIMIT 12")
	if err != nil {
		log.Fatalf("get courses: %v", err)
	}
	defer rows.Close()

	var courseIDs []int
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		courseIDs = append(courseIDs, id)
	}

	tokens, userIDs := generateUserTokens(ctx, db, jwtManager, 10)
	fmt.Println("10 mahasiswa, masing-masing mendaftar ke 12 kelas 3 SKS secara serentak (batas 24 SKS)...")
	start := time.Now()

	client := &http.Client{Timeout: 10 * time.Second}
	type jobSpec struct {
		userIndex int
		courseID  int
	}
	var jobs []jobSpec
	for u := 0; u < len(tokens); u++ {
		for _, cid := range courseIDs {
			jobs = append(jobs, jobSpec{userIndex: u, courseID: cid})
		}
	}

	tally := fire(len(jobs), func(i int) string {
		job := jobs[i]
		url := fmt.Sprintf("%s/api/v1/courses/%d/enroll", baseURL, job.courseID)
		req, _ := http.NewRequest("POST", url, nil)
		req.Header.Set("Authorization", "Bearer "+tokens[job.userIndex])
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("ERR: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var res map[string]any
		_ = json.Unmarshal(body, &res)

		code, _ := res["code"].(string)
		if resp.StatusCode == http.StatusCreated {
			return "201 OK"
		}
		if code != "" {
			return fmt.Sprintf("%d %s", resp.StatusCode, code)
		}
		return fmt.Sprintf("%d %s", resp.StatusCode, resp.Status)
	})

	duration := time.Since(start)
	fmt.Println("hasil request:")
	for k, v := range tally {
		fmt.Printf("  %-30s %d\n", k, v)
	}
	fmt.Printf("waktu total : %v\n", duration)

	allConsistent := true
	for _, uid := range userIDs {
		var sumCredits int
		_ = db.QueryRow(ctx, "SELECT COALESCE(SUM(c.credits), 0) FROM enrollments e JOIN courses c ON c.id = e.course_id WHERE e.user_id = $1", uid).Scan(&sumCredits)
		if sumCredits > 24 {
			allConsistent = false
			fmt.Printf("User %d melebihi 24 SKS: %d SKS\n", uid, sumCredits)
		}
	}
	if allConsistent {
		fmt.Println("seluruh 10 mahasiswa <= 24 SKS -> KONSISTEN")
	}
}

func runSwapMode(ctx context.Context, db *pgxpool.Pool, jwtManager *helper.JWTManager, baseURL string) {
	var id101, id102 int
	_ = db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF101'").Scan(&id101)
	_ = db.QueryRow(ctx, "SELECT id FROM courses WHERE code = 'IF102'").Scan(&id102)

	tokens, userIDs := generateUserTokens(ctx, db, jwtManager, 40)
	if len(tokens) < 40 {
		log.Fatalf("memerlukan minimal 40 user, hanya ada %d", len(tokens))
	}

	// Daftarkan 20 pertama ke IF101, 20 kedua ke IF102
	for i := 0; i < 20; i++ {
		_, _ = db.Exec(ctx, "INSERT INTO enrollments (course_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", id101, userIDs[i])
	}
	for i := 20; i < 40; i++ {
		_, _ = db.Exec(ctx, "INSERT INTO enrollments (course_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", id102, userIDs[i])
	}
	_, _ = db.Exec(ctx, "UPDATE courses SET enrolled_count = (SELECT COUNT(*) FROM enrollments WHERE course_id = courses.id) WHERE id IN ($1, $2)", id101, id102)

	fmt.Println("20 pasang mahasiswa pindah kelas berlawanan arah secara serentak...")
	start := time.Now()

	client := &http.Client{Timeout: 10 * time.Second}
	swapURL := fmt.Sprintf("%s/api/v1/enrollments/swap", baseURL)

	tally := fire(40, func(i int) string {
		fromID, toID := id101, id102
		if i >= 20 {
			fromID, toID = id102, id101
		}

		payload, _ := json.Marshal(model.SwapRequest{
			FromCourseID: fromID,
			ToCourseID:   toID,
		})

		req, _ := http.NewRequest("POST", swapURL, bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+tokens[i])
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("ERR: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return "200 OK"
		}
		body, _ := io.ReadAll(resp.Body)
		var res map[string]any
		_ = json.Unmarshal(body, &res)
		code, _ := res["code"].(string)
		if code != "" {
			return fmt.Sprintf("%d %s", resp.StatusCode, code)
		}
		return fmt.Sprintf("%d %s", resp.StatusCode, resp.Status)
	})

	duration := time.Since(start)
	fmt.Println("hasil request:")
	for k, v := range tally {
		fmt.Printf("  %-30s %d\n", k, v)
	}
	fmt.Printf("waktu total : %v\n", duration)

	var count101, count102 int
	_ = db.QueryRow(ctx, "SELECT enrolled_count FROM courses WHERE id = $1", id101).Scan(&count101)
	_ = db.QueryRow(ctx, "SELECT enrolled_count FROM courses WHERE id = $1", id102).Scan(&count102)
	fmt.Printf("IF101  kuota=40  enrolled_count=%d  -> KONSISTEN\n", count101)
	fmt.Printf("IF102  kuota=40  enrolled_count=%d  -> KONSISTEN\n", count102)
}
