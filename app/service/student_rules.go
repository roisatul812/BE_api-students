package service

import (
	model "api-students/app/model"
)

// ApplyPatch menerapkan field PATCH yang dikirim ke data student.
// Seluruh validasi field telah dilakukan secara deklaratif melalui struct tag
// sebelum fungsi ini dipanggil (Modul 7 D.2).
func ApplyPatch(
	current model.Student,
	req model.PatchStudentRequest,
) model.Student {
	if req.NIM != nil {
		current.NIM = *req.NIM
	}

	if req.Name != nil {
		current.Name = *req.Name
	}

	if req.Grade != nil {
		current.Grade = int(*req.Grade)
	}

	if req.IsActive != nil {
		current.IsActive = *req.IsActive
	}

	return current
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
