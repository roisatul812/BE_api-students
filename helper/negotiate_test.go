package helper_test

import (
	"bytes"
	"encoding/csv"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	model "api-students/app/model"
	"api-students/helper"

	"github.com/gofiber/fiber/v2"
)

func TestCSVWriter_PlantedVsFixed(t *testing.T) {
	dummyStudents := []model.Student{
		{
			ID:        1,
			NIM:       "240001",
			Name:      "Budi Santoso",
			Grade:     90,
			IsActive:  true,
			OwnerID:   1,
			CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:        2,
			NIM:       "240002",
			Name:      "Siti Rahma",
			Grade:     95,
			IsActive:  true,
			OwnerID:   1,
			CreatedAt: time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC),
		},
	}

	// 1. REPRODUKSI PLANTED ERROR LANGKAH 8:
	// Membuktikan bahwa tanpa writer.Flush(), buffer internal csv.Writer
	// tidak menulis ke underlying bytes.Buffer sehingga output kosong (0 bytes)!
	plantedBytes, err := helper.PlantedGenerateStudentsCSV(dummyStudents)
	if err != nil {
		t.Fatalf("unexpected error in planted csv generator: %v", err)
	}
	t.Logf("PLANTED ERROR REPRODUKSI: output CSV length = %d bytes (data tertahan di internal buffer tanpa Flush)", len(plantedBytes))
	if len(plantedBytes) != 0 {
		t.Errorf("planted error seharusnya menghasilkan 0 bytes karena Flush() tidak dipanggil, tapi menghasilkan %d bytes", len(plantedBytes))
	}

	// 2. PEMBUKTIAN PERBAIKAN BUG #9:
	// Memanggil writer.Flush() dan mengecek writer.Error() menghasilkan data CSV lengkap!
	fixedBytes, err := helper.GenerateStudentsCSV(dummyStudents)
	if err != nil {
		t.Fatalf("fixed csv generator returned error: %v", err)
	}
	t.Logf("FIXED BUG #9 HASIL AKTUAL: output CSV length = %d bytes\nIsi CSV:\n%s", len(fixedBytes), string(fixedBytes))

	if len(fixedBytes) == 0 {
		t.Fatalf("fixed CSV generator menghasilkan output kosong!")
	}

	// Validasi isi CSV
	reader := csv.NewReader(bytes.NewReader(fixedBytes))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("gagal membaca output CSV: %v", err)
	}

	// Header + 2 baris data = 3 baris
	if len(records) != 3 {
		t.Errorf("expected 3 records (1 header + 2 data), got %d", len(records))
	}
	if records[0][0] != "id" || records[0][1] != "nim" || records[0][2] != "name" {
		t.Errorf("header CSV tidak valid: %v", records[0])
	}
	if records[1][1] != "240001" || records[2][1] != "240002" {
		t.Errorf("data CSV tidak sesuai: %v, %v", records[1], records[2])
	}
}

func TestContentNegotiation(t *testing.T) {
	app := fiber.New()

	app.Get("/test-negotiate", func(c *fiber.Ctx) error {
		format, err := helper.Negotiate(c, helper.FormatJSON, helper.FormatCSV)
		if err != nil {
			var appErr *helper.AppError
			if errors.As(err, &appErr) {
				return c.Status(appErr.Status).JSON(fiber.Map{
					"code":    appErr.Code,
					"message": appErr.Message,
				})
			}
			return err
		}
		return c.SendString(format)
	})

	// Test 1: Accept: application/json -> 200 (application/json)
	reqJSON := httptest.NewRequest("GET", "/test-negotiate", nil)
	reqJSON.Header.Set("Accept", "application/json")
	respJSON, err := app.Test(reqJSON)
	if err != nil || respJSON.StatusCode != 200 {
		t.Errorf("expected 200 for application/json, got status=%d err=%v", respJSON.StatusCode, err)
	}

	// Test 2: Accept: text/csv -> 200 (text/csv)
	reqCSV := httptest.NewRequest("GET", "/test-negotiate", nil)
	reqCSV.Header.Set("Accept", "text/csv")
	respCSV, err := app.Test(reqCSV)
	if err != nil || respCSV.StatusCode != 200 {
		t.Errorf("expected 200 for text/csv, got status=%d err=%v", respCSV.StatusCode, err)
	}

	// Test 3: Accept: */* -> 200 (default FormatJSON)
	reqAll := httptest.NewRequest("GET", "/test-negotiate", nil)
	reqAll.Header.Set("Accept", "*/*")
	respAll, err := app.Test(reqAll)
	if err != nil || respAll.StatusCode != 200 {
		t.Errorf("expected 200 for */*, got status=%d err=%v", respAll.StatusCode, err)
	}

	// Test 4: Tanpa Accept header -> 200 (default FormatJSON)
	reqEmpty := httptest.NewRequest("GET", "/test-negotiate", nil)
	respEmpty, err := app.Test(reqEmpty)
	if err != nil || respEmpty.StatusCode != 200 {
		t.Errorf("expected 200 for empty Accept, got status=%d err=%v", respEmpty.StatusCode, err)
	}

	// Test 5: Accept: application/xml (Unsupported -> 406 Not Acceptable)
	reqXML := httptest.NewRequest("GET", "/test-negotiate", nil)
	reqXML.Header.Set("Accept", "application/xml")
	respXML, err := app.Test(reqXML)
	if err != nil || respXML.StatusCode != fiber.StatusNotAcceptable {
		t.Errorf("expected 406 for application/xml, got status=%d err=%v", respXML.StatusCode, err)
	}
	t.Logf("Content Negotiation 406 Not Acceptable verified for application/xml: status=%d", respXML.StatusCode)
}
