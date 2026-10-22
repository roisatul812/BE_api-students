package helper_test

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"api-students/helper"
)

func TestCursor_EncodeDecode_Success(t *testing.T) {
	now := time.Now().UTC()
	id := 42

	encoded := helper.EncodeCursor(now, id)
	if encoded == "" {
		t.Fatalf("EncodeCursor returned empty string")
	}

	decoded, err := helper.DecodeCursor(encoded)
	if err != nil {
		t.Fatalf("DecodeCursor returned unexpected error: %v", err)
	}

	if decoded.ID != id {
		t.Errorf("expected ID %d, got %d", id, decoded.ID)
	}

	// Bandingkan timestamp (dalam presisi mikrodetik/nanodetik yang sama)
	if !decoded.CreatedAt.Equal(now) {
		t.Errorf("expected CreatedAt %v, got %v", now, decoded.CreatedAt)
	}
}

func TestCursor_Decode_InvalidCases(t *testing.T) {
	invalidCases := map[string]string{
		"":                               "string kosong",
		"   ":                            "whitespace saja",
		"abc":                            "base64 tidak valid / payload tidak lengkap",
		"bukan-base64!!":                 "karakter non-base64",
		base64.RawURLEncoding.EncodeToString([]byte("single_part_without_pipe")):           "tidak ada pemisah pipe",
		base64.RawURLEncoding.EncodeToString([]byte("part1|part2|part3")):                 "lebih dari 2 part",
		base64.RawURLEncoding.EncodeToString([]byte("invalid-time-format|15")):            "timestamp bukan format RFC3339",
		base64.RawURLEncoding.EncodeToString([]byte("2026-09-27T10:00:00Z|bukan_angka")):  "ID bukan angka",
		base64.RawURLEncoding.EncodeToString([]byte("2026-09-27T10:00:00Z|0")):            "ID nol (bukan positive int)",
		base64.RawURLEncoding.EncodeToString([]byte("2026-09-27T10:00:00Z|-10")):          "ID negatif",
	}

	for cursorStr, desc := range invalidCases {
		decoded, err := helper.DecodeCursor(cursorStr)
		if err == nil {
			t.Errorf("case '%s' (%s) seharusnya mengembalikan error, tapi berhasil: %+v", cursorStr, desc, decoded)
			continue
		}

		var appErr *helper.AppError
		if !errors.As(err, &appErr) {
			t.Errorf("case '%s': expected *helper.AppError, got %T: %v", cursorStr, err, err)
			continue
		}

		if appErr.Status != 400 || appErr.Code != "BAD_REQUEST" {
			t.Errorf("case '%s': expected Status 400 BAD_REQUEST, got status=%d code=%s", cursorStr, appErr.Status, appErr.Code)
		}
	}
}
