package service_test

import (
	"testing"

	"api-students/app/service"
)

func TestWithinCreditLimit(t *testing.T) {
	tests := []struct {
		name     string
		taken    int
		adding   int
		expected bool
	}{
		{"empty krs taking 3 sks", 0, 3, true},
		{"taking 21 sks then adding 3 sks", 21, 3, true},
		{"taking 22 sks then adding 3 sks exceeds 24", 22, 3, false},
		{"taking 24 sks then adding 1 sks exceeds 24", 24, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.WithinCreditLimit(tt.taken, tt.adding)
			if got != tt.expected {
				t.Errorf("WithinCreditLimit(%d, %d) = %v; want %v", tt.taken, tt.adding, got, tt.expected)
			}
		})
	}
}

func TestCreditsAfterSwap(t *testing.T) {
	tests := []struct {
		name     string
		taken    int
		leaving  int
		joining  int
		expected int
	}{
		{"same credits swap", 21, 3, 3, 21},
		{"drop 3 add 2", 24, 3, 2, 23},
		{"drop 2 add 4", 20, 2, 4, 22},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.CreditsAfterSwap(tt.taken, tt.leaving, tt.joining)
			if got != tt.expected {
				t.Errorf("CreditsAfterSwap(%d, %d, %d) = %d; want %d", tt.taken, tt.leaving, tt.joining, got, tt.expected)
			}
		})
	}
}
