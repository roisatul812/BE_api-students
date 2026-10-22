package helper

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int       `json:"id"`
}

// EncodeCursor mengonversi created_at dan id menjadi string base64 RawURL yang aman untuk URL.
// Menggunakan waktu UTC dan pemisah pipe "|" (Modul 7 D.3).
func EncodeCursor(createdAt time.Time, id int) string {
	payload := fmt.Sprintf("%s|%d", createdAt.UTC().Format(time.RFC3339Nano), id)
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// DecodeCursor mendekode string base64 RawURL menjadi struct Cursor.
// Menolak cursor kosong, base64 invalid, bagian tidak tepat 2, timestamp invalid, dan ID invalid.
// Menghasilkan *AppError HTTP 400 (Bad Request).
func DecodeCursor(encoded string) (*Cursor, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, BadRequest("cursor tidak boleh kosong")
	}

	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		// Fallback jika menggunakan padding standar
		var errPad error
		b, errPad = base64.URLEncoding.DecodeString(encoded)
		if errPad != nil {
			return nil, BadRequest("format cursor tidak valid (bukan base64 URL)")
		}
	}

	parts := strings.Split(string(b), "|")
	if len(parts) != 2 {
		return nil, BadRequest("format cursor tidak valid (harus memuat timestamp dan id)")
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		createdAt, err = time.Parse(time.RFC3339, parts[0])
		if err != nil {
			return nil, BadRequest("timestamp pada cursor tidak valid")
		}
	}

	id, err := strconv.Atoi(parts[1])
	if err != nil || id <= 0 {
		return nil, BadRequest("id pada cursor tidak valid")
	}

	return &Cursor{
		CreatedAt: createdAt.UTC(),
		ID:        id,
	}, nil
}
