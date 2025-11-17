package boxoffice

import (
	"testing"
	"time"
)

func FuzzConvertToResult(f *testing.F) {
	f.Add(int64(1000000), int64(500000), "Warner Bros", "USD", "BoxAPI")

	f.Fuzz(func(t *testing.T, worldwide, budget int64, distributor, currency, source string) {
		resp := apiResponse{
			Distributor: optionalString(distributor),
			Budget:      &budget,
			Revenue: revenuePayload{
				Worldwide: &worldwide,
			},
		}
		if worldwide%2 == 0 {
			resp.Revenue.Worldwide = nil
		}

		if currency != "" {
			resp.Currency = &currency
		}
		if source != "" {
			resp.Source = &source
		}

		last := time.Unix(worldwide, 0)
		resp.LastUpdated = &last

		result := convertToResult(resp)
		if result == nil || result.BoxOffice == nil {
			t.Fatalf("convertToResult returned nil result")
		}
		if result.BoxOffice.Currency == "" {
			t.Fatalf("currency should never be empty")
		}
		if result.BoxOffice.Source == "" {
			t.Fatalf("source should never be empty")
		}
	})
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
