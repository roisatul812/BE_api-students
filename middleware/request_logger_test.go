package middleware_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"

	"api-students/config"
	"api-students/helper"
	"api-students/middleware"

	"github.com/gofiber/fiber/v2"
)

// TestRequestLogger_PlantedError membuktikan bahwa membaca c.Response().StatusCode()
// secara langsung pada middleware akan mencatat status 200 untuk request yang mengembalikan error.
func TestRequestLogger_StatusCorrection(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	app := fiber.New(fiber.Config{
		ErrorHandler: config.NewErrorHandler(logger),
	})

	app.Use(middleware.RequestLogger(logger))

	// Endpoint yang mengembalikan AppError (Langkah 1 & 2 Modul 7)
	app.Get("/test-404", func(c *fiber.Ctx) error {
		return helper.NotFound("data tidak ditemukan")
	})

	app.Get("/test-500", func(c *fiber.Ctx) error {
		return helper.Internal(fiber.ErrInternalServerError)
	})

	app.Get("/test-200", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	// 1. Test Endpoint 404
	req := httptest.NewRequest("GET", "/test-404", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("expected HTTP 404, got %d", resp.StatusCode)
	}

	// Periksa log entry terakhir
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	var lastLog map[string]any
	found := false
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err == nil {
			if entry["msg"] == "http_request" && entry["path"] == "/test-404" {
				lastLog = entry
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("log http_request harus ditemukan di: %s", buf.String())
	}

	loggedStatus, ok := lastLog["status"].(float64)
	if !ok {
		t.Fatalf("field status bukan number: %v", lastLog["status"])
	}
	t.Logf("HASIL AKTUAL LOG: logged status = %v (HTTP response = %d)", loggedStatus, resp.StatusCode)

	if loggedStatus != 404 {
		t.Errorf("PLANTED ERROR TERBUKTI! RequestLogger mencatat status %v padahal response adalah %d", loggedStatus, resp.StatusCode)
	}
}
