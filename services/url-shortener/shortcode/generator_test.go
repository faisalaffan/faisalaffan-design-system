package shortcode

import "testing"

func TestGenerate(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code, err := Generate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != 7 {
			t.Errorf("expected length 7, got %d (%s)", len(code), code)
		}
		if seen[code] {
			t.Errorf("duplicate code: %s", code)
		}
		seen[code] = true
	}
}

func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Generate()
	}
}
