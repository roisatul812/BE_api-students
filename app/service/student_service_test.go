package service_test

import (
	"errors"
	"testing"

	"api-students/app/service"
	"api-students/helper"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PlantedTranslateError merekonstruksi planted error Langkah 4 Modul 7,
// di mana default clause mengembalikan nil untuk unknown database error.
func PlantedTranslateError(err error, entity string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return helper.NotFound(entity + " tidak ditemukan")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return helper.Conflict(entity + " dengan data tersebut sudah ada")
		case "23503":
			return helper.BadRequest("referensi data tidak valid")
		}
	}
	// PLANTED ERROR MODUL 7 LANGKAH 4:
	// default: return nil
	return nil
}

func TestTranslateError_PlantedVsFixed(t *testing.T) {
	unknownErr := errors.New("connection reset by peer / disk failure")

	// 1. REPRODUKSI PLANTED ERROR:
	// Membuktikan bahwa default: return nil menelan unknown database error
	plantedResult := PlantedTranslateError(unknownErr, "student")
	t.Logf("PLANTED ERROR REPRODUKSI: unknownErr=%v -> result=%v (error tertelan menjadi nil!)", unknownErr, plantedResult)
	if plantedResult != nil {
		t.Errorf("planted error seharusnya mengembalikan nil (menelan error), tapi mengembalikan: %v", plantedResult)
	}

	// 2. PEMBUKTIAN PERBAIKAN BUG #5:
	// Implementasi fix harus fail-closed: mengembalikan AppError INTERNAL_ERROR (500)
	fixedResult := service.TranslateError(unknownErr, "student")
	if fixedResult == nil {
		t.Fatalf("BUG #5 FIX GAGAL: TranslateError masih mengembalikan nil untuk unknown database error!")
	}

	var appErr *helper.AppError
	if !errors.As(fixedResult, &appErr) {
		t.Fatalf("expected *helper.AppError, got %T: %v", fixedResult, fixedResult)
	}

	if appErr.Status != 500 || appErr.Code != "INTERNAL_ERROR" {
		t.Errorf("expected Status 500 INTERNAL_ERROR, got status=%d code=%s", appErr.Status, appErr.Code)
	}
	t.Logf("HASIL AKTUAL PERBAIKAN BUG #5: unknown error diterjemahkan ke Status=%d Code=%s Message=%s", appErr.Status, appErr.Code, appErr.Message)
}

func TestTranslateError_KnownErrors(t *testing.T) {
	// Test pgx.ErrNoRows -> NOT_FOUND (404)
	errNoRows := service.TranslateError(pgx.ErrNoRows, "student")
	var notFoundAppErr *helper.AppError
	if !errors.As(errNoRows, &notFoundAppErr) || notFoundAppErr.Status != 404 || notFoundAppErr.Code != "NOT_FOUND" {
		t.Errorf("expected 404 NOT_FOUND, got: %v", errNoRows)
	}
	t.Logf("Known Error 1 (ErrNoRows): Status=%d Code=%s Message=%s", notFoundAppErr.Status, notFoundAppErr.Code, notFoundAppErr.Message)

	// Test unique violation 23505 -> CONFLICT (409)
	pgErrConflict := &pgconn.PgError{Code: "23505"}
	errConflict := service.TranslateError(pgErrConflict, "student")
	var conflictAppErr *helper.AppError
	if !errors.As(errConflict, &conflictAppErr) || conflictAppErr.Status != 409 || conflictAppErr.Code != "CONFLICT" {
		t.Errorf("expected 409 CONFLICT, got: %v", errConflict)
	}
	t.Logf("Known Error 2 (Unique 23505): Status=%d Code=%s Message=%s", conflictAppErr.Status, conflictAppErr.Code, conflictAppErr.Message)

	// Test nil error -> nil
	if errNil := service.TranslateError(nil, "student"); errNil != nil {
		t.Errorf("expected nil for nil input, got: %v", errNil)
	}
}
