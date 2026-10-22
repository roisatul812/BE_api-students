package helper_test

import (
	"testing"

	model "api-students/app/model"
	"api-students/helper"
)

func TestNIMCustomValidation_Valid(t *testing.T) {
	validCases := []string{
		"240003",
		"2400999",
		"24108888",
		"1234567890",
	}

	for _, nim := range validCases {
		req := model.CreateStudentRequest{
			NIM:      nim,
			Name:     "Valid Student",
			Grade:    85,
			IsActive: true,
		}
		errs := helper.ValidateStruct(req)
		if len(errs) > 0 {
			t.Errorf("NIM '%s' seharusnya valid, tapi ditolak: %v", nim, errs)
		}
	}
}

func TestNIMCustomValidation_Invalid(t *testing.T) {
	invalidCases := map[string]string{
		"12345":       "terlalu pendek (< 6 digit)",
		"12345678901": "terlalu panjang (> 10 digit)",
		"24000A":      "mengandung huruf",
		"24 0001":     "mengandung spasi",
		"24-0001":     "mengandung simbol",
		"":            "kosong",
	}

	for nim, desc := range invalidCases {
		req := model.CreateStudentRequest{
			NIM:      nim,
			Name:     "Test Student",
			Grade:    85,
			IsActive: true,
		}
		errs := helper.ValidateStruct(req)
		if len(errs) == 0 {
			t.Errorf("NIM '%s' (%s) seharusnya ditolak, tapi lolos validasi", nim, desc)
		}
		if nim != "" {
			expectedMsg := "format NIM harus berupa 6-10 digit angka"
			if errs["nim"] != expectedMsg {
				t.Errorf("NIM '%s' pesan error salah. Got: '%s', Expected: '%s'", nim, errs["nim"], expectedMsg)
			}
		}
	}
}

func TestPatchStudentRequest_DeclarativeValidation(t *testing.T) {
	// 1. TEST A: name dikirim kosong ("") -> HARUS DITOLAK
	emptyName := ""
	patchEmptyName := model.PatchStudentRequest{
		Name: &emptyName,
	}
	errs := helper.ValidateStruct(patchEmptyName)
	if len(errs) == 0 || errs["name"] == "" {
		t.Fatalf("PATCH name='' seharusnya ditolak, tapi lolos: %v", errs)
	}
	t.Logf("PATCH name='' berhasil ditolak secara deklaratif: %v", errs)

	// 2. TEST B: name tidak dikirim (omitted/nil), field lain valid -> HARUS DITERIMA
	validGrade := 92.5
	patchOmittedName := model.PatchStudentRequest{
		Grade: &validGrade,
	}
	errsOmitted := helper.ValidateStruct(patchOmittedName)
	if len(errsOmitted) > 0 {
		t.Fatalf("PATCH name omitted seharusnya diterima, tapi ditolak: %v", errsOmitted)
	}
	t.Logf("PATCH name omitted berhasil diterima secara deklaratif")

	// 3. TEST C: grade di luar rentang (> 100) -> HARUS DITOLAK
	invalidGrade := 105.0
	patchInvalidGrade := model.PatchStudentRequest{
		Grade: &invalidGrade,
	}
	errsGrade := helper.ValidateStruct(patchInvalidGrade)
	if len(errsGrade) == 0 || errsGrade["grade"] == "" {
		t.Fatalf("PATCH grade=105 seharusnya ditolak, tapi lolos: %v", errsGrade)
	}
	t.Logf("PATCH grade=105 berhasil ditolak secara deklaratif: %v", errsGrade)
}
