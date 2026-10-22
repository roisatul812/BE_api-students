package helper

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	model "api-students/app/model"

	"github.com/gofiber/fiber/v2"
)

const (
	FormatJSON          = "application/json"
	FormatCSV           = "text/csv"
	MIMEApplicationJSON = "application/json"
	MIMETextCSV         = "text/csv"
	MIMEAll             = "*/*"
)

// Negotiate memilih format response berdasarkan header Accept (Modul 7 Langkah 8 & D.4).
//
// Perbedaan yang wajib jelas:
//   - Content-Type menjelaskan format yang SEDANG DIKIRIM pengirim.
//   - Accept menjelaskan format yang DIINGINKAN penerima sebagai balasan.
func Negotiate(c *fiber.Ctx, offered ...string) (string, error) {
	accept := strings.TrimSpace(c.Get(fiber.HeaderAccept))

	// Tidak menyebut Accept sama sekali berarti "terserah server".
	// Demikian pula Accept: */* yang dikirim hampir semua tool CLI.
	if accept == "" || accept == "*/*" {
		return offered[0], nil
	}

	chosen := c.Accepts(offered...)
	if chosen == "" {
		return "", NotAcceptable(
			"format yang diminta tidak tersedia, pilih salah satu dari: " +
				strings.Join(offered, ", "))
	}

	return chosen, nil
}

// NegotiateContentType memeriksa header Accept dan mengembalikan format yang diminta.
// Wrapper kompatibilitas yang memanggil Negotiate.
func NegotiateContentType(c *fiber.Ctx) (string, error) {
	return Negotiate(c, FormatJSON, FormatCSV)
}

// GenerateStudentsCSV mengonversi slice Student menjadi CSV dengan writer.Flush() dan pengecekan writer.Error().
func GenerateStudentsCSV(students []model.Student) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	header := []string{"id", "nim", "name", "grade", "is_active", "owner_id", "created_at"}
	if err := writer.Write(header); err != nil {
		return nil, Internal(err)
	}

	for _, s := range students {
		record := []string{
			fmt.Sprintf("%d", s.ID),
			s.NIM,
			s.Name,
			fmt.Sprintf("%d", s.Grade),
			fmt.Sprintf("%t", s.IsActive),
			fmt.Sprintf("%d", s.OwnerID),
			s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if err := writer.Write(record); err != nil {
			return nil, Internal(err)
		}
	}

	// PERBAIKAN BUG #9:
	// Memanggil Flush() sebelum buffer dikirim dan memverifikasi writer.Error().
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, Internal(err)
	}

	return buf.Bytes(), nil
}

// PlantedGenerateStudentsCSV merekonstruksi planted error Modul 7 Langkah 8,
// di mana writer.Flush() dihilangkan sehingga data tertahan di buffer internal csv.Writer.
func PlantedGenerateStudentsCSV(students []model.Student) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	header := []string{"id", "nim", "name", "grade", "is_active", "owner_id", "created_at"}
	_ = writer.Write(header)

	for _, s := range students {
		record := []string{
			fmt.Sprintf("%d", s.ID),
			s.NIM,
			s.Name,
			fmt.Sprintf("%d", s.Grade),
			fmt.Sprintf("%t", s.IsActive),
			fmt.Sprintf("%d", s.OwnerID),
			s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		_ = writer.Write(record)
	}

	// PLANTED ERROR MODUL 7 LANGKAH 8: Tidak memanggil writer.Flush()!
	return buf.Bytes(), nil
}
