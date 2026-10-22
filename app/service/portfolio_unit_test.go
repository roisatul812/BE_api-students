package service_test

import (
	"encoding/json"
	"testing"

	"api-students/app/model"
	"api-students/app/service"
	"api-students/helper"
)

func TestNormalizeTechnologies(t *testing.T) {
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
			input:    []string{"  Golang ", "PostgreSQL", " Docker  "},
			expected: []string{"golang", "postgresql", "docker"},
		},
		{
			name:     "deduplicate preserving order",
			input:    []string{"React", "react", "REACT", "Node.js", "react"},
			expected: []string{"react", "node.js"},
		},
		{
			name:     "ignore empty elements",
			input:    []string{"", "   ", "kubernetes", " "},
			expected: []string{"kubernetes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.NormalizeTechnologies(tt.input)
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

func TestValidateUpsertPortfolioRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := model.UpsertPortfolioRequest{
			Headline: "Backend Developer & Cloud Enthusiast",
			Bio:      "Mahasiswa D4 Teknik Informatika dengan ketertarikan kuat pada sistem terdistribusi dan database.",
			Projects: []model.CreateProjectInput{
				{
					Title:        "Sistem Akademik API",
					Technologies: []string{"Go", "PostgreSQL", "Docker"},
					Link:         "https://github.com/example/api",
					Year:         2024,
				},
			},
			Certificates: []model.CreateCertInput{
				{
					Name:   "AWS Certified Cloud Practitioner",
					Issuer: "Amazon Web Services",
					Date:   "2024-05-15",
				},
			},
		}
		errs := helper.ValidateStruct(req)
		if len(errs) != 0 {
			t.Fatalf("seharusnya valid, got errors: %v", errs)
		}
	})

	t.Run("headline too short", func(t *testing.T) {
		req := model.UpsertPortfolioRequest{
			Headline: "Dev", // < 5 karakter
			Bio:      "Deskripsi biografi yang panjang dan valid untuk memenuhi syarat.",
		}
		errs := helper.ValidateStruct(req)
		if errs["headline"] != "minimal 5 karakter" {
			t.Errorf("pesan error salah, got: %q", errs["headline"])
		}
	})

	t.Run("bio too short", func(t *testing.T) {
		req := model.UpsertPortfolioRequest{
			Headline: "Software Engineer",
			Bio:      "Pendek", // < 10 karakter
		}
		errs := helper.ValidateStruct(req)
		if errs["bio"] != "minimal 10 karakter" {
			t.Errorf("pesan error salah, got: %q", errs["bio"])
		}
	})

	t.Run("projects exceeding 20 items", func(t *testing.T) {
		projs := make([]model.CreateProjectInput, 21)
		for i := 0; i < 21; i++ {
			projs[i] = model.CreateProjectInput{
				Title:        "Project Title",
				Technologies: []string{"Go"},
				Year:         2024,
			}
		}
		req := model.UpsertPortfolioRequest{
			Headline: "Backend Engineer",
			Bio:      "Biografi yang panjang dan valid untuk portofolio mahasiswa.",
			Projects: projs,
		}
		errs := helper.ValidateStruct(req)
		if errs["projects"] != "maksimal 20 item" {
			t.Errorf("seharusnya maksimal 20 item, got: %q", errs["projects"])
		}
	})
}

func TestValidateCreateProjectRequest(t *testing.T) {
	t.Run("valid project", func(t *testing.T) {
		req := model.CreateProjectRequest{
			Title:        "Aplikasi Mobile Kasir",
			Technologies: []string{"Flutter", "Firebase"},
			Link:         "https://example.com/app",
			Year:         2023,
		}
		errs := helper.ValidateStruct(req)
		if len(errs) != 0 {
			t.Fatalf("seharusnya valid, got errors: %v", errs)
		}
	})

	t.Run("empty technologies", func(t *testing.T) {
		req := model.CreateProjectRequest{
			Title:        "Project Valid",
			Technologies: []string{}, // min 1
			Year:         2024,
		}
		errs := helper.ValidateStruct(req)
		if len(errs) == 0 || errs["technologies"] == "" {
			t.Errorf("technologies kosong harus menghasilkan error validasi")
		}
	})

	t.Run("invalid year", func(t *testing.T) {
		req := model.CreateProjectRequest{
			Title:        "Project Valid",
			Technologies: []string{"Go"},
			Year:         1990, // < 2000
		}
		errs := helper.ValidateStruct(req)
		if len(errs) == 0 || errs["year"] == "" {
			t.Errorf("year 1990 harus menghasilkan error validasi")
		}
	})
}

func TestPortfolioNoSQLInjectionPayloadPrevention(t *testing.T) {
	rawJSON := `{"headline": {"$ne": ""}, "bio": "Valid bio text that is long enough"}`
	var req model.UpsertPortfolioRequest
	err := json.Unmarshal([]byte(rawJSON), &req)
	if err == nil {
		t.Fatalf("seharusnya gagal unmarshal karena headline bertipe string, bukan object operator NoSQL")
	}
}
