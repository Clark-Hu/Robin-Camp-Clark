package httpserver

import (
	"math"
	"testing"
)

func TestRoundToOneDecimal(t *testing.T) {
	tests := []struct {
		name  string
		value float32
		want  float32
	}{
		{"zero", 0, 0},
		{"round-up", 3.75, 3.8},
		{"round-down", 2.74, 2.7},
		{"exact", 4.5, 4.5},
		{"large", 199.94, 199.9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundToOneDecimal(tt.value)
			if math.Abs(float64(got-tt.want)) > 0.0001 {
				t.Fatalf("roundToOneDecimal(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestAllowedRatings(t *testing.T) {
	valid := []float32{0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0}
	for _, rating := range valid {
		if _, ok := allowedRatings[rating]; !ok {
			t.Fatalf("rating %v should be allowed", rating)
		}
	}

	invalid := []float32{0, 0.25, 3.7, 5.5}
	for _, rating := range invalid {
		if _, ok := allowedRatings[rating]; ok {
			t.Fatalf("rating %v should not be allowed", rating)
		}
	}
}
