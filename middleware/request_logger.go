package middleware

import (
	"errors"
	"log/slog"
	"time"

	"api-students/helper"

	"github.com/gofiber/fiber/v2"
)

// RequestLogger mencatat access log untuk setiap request yang masuk.
// Planted error dari Langkah 3: membaca c.Response().StatusCode() langsung,
// sehingga saat handler mengembalikan error, status tercatat sebagai 200.
func RequestLogger(logger *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		requestID := helper.RequestID(c)

		// Sejak handler mengembalikan error alih-alih menulis response sendiri,
		// status pada c.Response() BELUM terisi ketika baris ini dijalankan:
		// ErrorHandler baru berjalan setelah seluruh rangkaian middleware selesai.
		// Tanpa koreksi di bawah, setiap kegagalan tercatat sebagai 200.
		status := c.Response().StatusCode()
		if err != nil {
			var appErr *helper.AppError
			if errors.As(err, &appErr) {
				status = appErr.Status
			} else {
				status = fiber.StatusInternalServerError
			}
		}
		// PERBAIKAN BUG #4 (Modul 7 Langkah 3):
		// Menggunakan variabel status yang telah dikoreksi dari AppError/ErrorHandler,
		// bukan c.Response().StatusCode() yang masih bernilai 200 sebelum ErrorHandler berjalan.
		attrs := []any{
			slog.String("request_id", requestID),
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status), // FIXED!
			slog.Duration("duration", time.Since(start)),
			slog.String("ip", c.IP()),
		}

		logger.Info("http_request", attrs...)
		return err
	}
}
