package service

import (
	"strings"

	model "api-students/app/model"
)

// ValidateCreate memeriksa data student untuk proses POST.
func ValidateCreate(req model.CreateStudentRequest) map[string]string {
	errs := map[string]string{}

	if strings.TrimSpace(req.NIM) == "" {
		errs["nim"] = "NIM tidak boleh kosong"
	}

	if strings.TrimSpace(req.Name) == "" {
		errs["name"] = "nama tidak boleh kosong"
	}

	if req.Grade < 0 || req.Grade > 100 {
		errs["grade"] = "grade harus berada di antara 0 dan 100"
	}

	return errs
}

// ValidateReplace memeriksa data student untuk proses PUT.
func ValidateReplace(req model.ReplaceStudentRequest) map[string]string {
	errs := map[string]string{}

	if strings.TrimSpace(req.NIM) == "" {
		errs["nim"] = "NIM tidak boleh kosong"
	}

	if strings.TrimSpace(req.Name) == "" {
		errs["name"] = "nama tidak boleh kosong"
	}

	if req.Grade < 0 || req.Grade > 100 {
		errs["grade"] = "grade harus berada di antara 0 dan 100"
	}

	return errs
}

// ApplyPatch menerapkan field PATCH yang dikirim ke data student.
func ApplyPatch(
	current model.Student,
	req model.PatchStudentRequest,
) (model.Student, map[string]string) {

	errs := map[string]string{}

	if req.NIM != nil {
		if strings.TrimSpace(*req.NIM) == "" {
			errs["nim"] = "NIM tidak boleh kosong"
		} else {
			current.NIM = *req.NIM
		}
	}

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			errs["name"] = "nama tidak boleh kosong"
		} else {
			current.Name = *req.Name
		}
	}

	if req.Grade != nil {
		if *req.Grade < 0 || *req.Grade > 100 {
			errs["grade"] = "grade harus berada di antara 0 dan 100"
		} else {
			current.Grade = int(*req.Grade)
		}
	}

	if req.IsActive != nil {
		current.IsActive = *req.IsActive
	}

	return current, errs
}

// IsEmptyPatch mengecek apakah PATCH tidak mengirim field apa pun.
func IsEmptyPatch(req model.PatchStudentRequest) bool {
	return req.NIM == nil &&
		req.Name == nil &&
		req.Grade == nil &&
		req.IsActive == nil
}

// CountTotalPages menghitung jumlah halaman.
func CountTotalPages(total, limit int) int {
	if limit <= 0 {
		return 0
	}

	return (total + limit - 1) / limit
}
