package service_test

import (
	"encoding/json"
	"testing"

	"api-students/app/model"
	"api-students/app/service"
	"api-students/helper"
)

func TestNormalizeTags(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "trim and lowercase",
			input:    []string{"  Golang  ", "DOCKER ", "  Kubernetes"},
			expected: []string{"golang", "docker", "kubernetes"},
		},
		{
			name:     "deduplicate preserving order",
			input:    []string{"Tugas-Banyak", " dosen-jelas ", "tugas-banyak", "praktikal", "Tugas-Banyak"},
			expected: []string{"tugas-banyak", "dosen-jelas", "praktikal"},
		},
		{
			name:     "ignore empty after trim",
			input:    []string{"", "   ", "valid-tag", "   "},
			expected: []string{"valid-tag"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.NormalizeTags(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("panjang berbeda: got %d, expected %d", len(got), len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("elemen %d: got %q, expected %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestValidateCreateReviewRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := model.CreateReviewRequest{
			Rating:  5,
			Comment: "Materi sangat jelas dan praktikum menantang",
			Tags:    []string{"dosen-jelas", "praktikal"},
		}
		errs := helper.ValidateStruct(req)
		if len(errs) != 0 {
			t.Fatalf("seharusnya valid, got errors: %v", errs)
		}
	})

	t.Run("rating invalid (too low and too high)", func(t *testing.T) {
		low := model.CreateReviewRequest{
			Rating:  0,
			Comment: "Materi sangat jelas dan praktikum menantang",
		}
		errsLow := helper.ValidateStruct(low)
		if errsLow["rating"] == "" {
			t.Errorf("rating 0 harus menghasilkan error validasi")
		}

		high := model.CreateReviewRequest{
			Rating:  6,
			Comment: "Materi sangat jelas dan praktikum menantang",
		}
		errsHigh := helper.ValidateStruct(high)
		if errsHigh["rating"] == "" {
			t.Errorf("rating 6 harus menghasilkan error validasi")
		}
	})

	t.Run("comment too short", func(t *testing.T) {
		short := model.CreateReviewRequest{
			Rating:  4,
			Comment: "Pendek", // < 10 karakter
		}
		errs := helper.ValidateStruct(short)
		if errs["comment"] != "minimal 10 karakter" {
			t.Errorf("pesan error salah, got: %q", errs["comment"])
		}
	})

	t.Run("tags exceeding 5 items", func(t *testing.T) {
		req := model.CreateReviewRequest{
			Rating:  4,
			Comment: "Komentar yang cukup panjang untuk memenuhi syarat minimal 10 karakter",
			Tags:    []string{"tag1", "tag2", "tag3", "tag4", "tag5", "tag6"},
		}
		errs := helper.ValidateStruct(req)
		if errs["tags"] != "maksimal 5 item" {
			t.Errorf("seharusnya maksimal 5 item, got: %q", errs["tags"])
		}
	})

	t.Run("tag item too short or too long", func(t *testing.T) {
		req := model.CreateReviewRequest{
			Rating:  4,
			Comment: "Komentar valid untuk ulasan mata kuliah",
			Tags:    []string{"a"}, // < 2 karakter
		}
		errs := helper.ValidateStruct(req)
		if len(errs) == 0 {
			t.Errorf("tag dengan 1 karakter harus gagal validasi dive")
		}
	})
}

func TestValidateUpdateReviewRequest(t *testing.T) {
	t.Run("valid partial update", func(t *testing.T) {
		rating := 4
		req := model.UpdateReviewRequest{
			Rating: &rating,
		}
		errs := helper.ValidateStruct(req)
		if len(errs) != 0 {
			t.Fatalf("seharusnya valid, got: %v", errs)
		}
	})

	t.Run("invalid rating in patch", func(t *testing.T) {
		rating := 10
		req := model.UpdateReviewRequest{
			Rating: &rating,
		}
		errs := helper.ValidateStruct(req)
		if errs["rating"] == "" {
			t.Errorf("rating 10 harus gagal validasi")
		}
	})
}

func TestValidateReplyRequest(t *testing.T) {
	t.Run("valid reply", func(t *testing.T) {
		req := model.ReplyRequest{
			Body: "Terima kasih atas ulasannya!",
		}
		errs := helper.ValidateStruct(req)
		if len(errs) != 0 {
			t.Fatalf("seharusnya valid, got: %v", errs)
		}
	})

	t.Run("reply too short", func(t *testing.T) {
		req := model.ReplyRequest{
			Body: "A",
		}
		errs := helper.ValidateStruct(req)
		if errs["body"] != "minimal 2 karakter" {
			t.Errorf("pesan error salah: got %q", errs["body"])
		}
	})
}

func TestNoSQLInjectionPayloadPrevention(t *testing.T) {
	// Jika payload NoSQL injection dikirim berupa object {"$ne": ""} untuk comment
	rawJSON := `{"rating": 5, "comment": {"$ne": ""}, "tags": ["test"]}`
	var req model.CreateReviewRequest
	err := json.Unmarshal([]byte(rawJSON), &req)
	if err == nil {
		t.Fatalf("seharusnya gagal unmarshal karena comment bertipe string, bukan JSON object")
	}
}
