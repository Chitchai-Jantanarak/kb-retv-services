package helpers

import "testing"

func TestCompressRoundTrip(t *testing.T) {
	input := []byte("shared helper compression")

	compressed := Compress(input)
	if len(compressed) == 0 {
		t.Fatal("expected compressed bytes")
	}

	decompressed := Decompress(compressed)
	if decompressed == nil {
		t.Fatal("expected decompressed string")
	}
	if *decompressed != string(input) {
		t.Fatalf("expected %q, got %q", string(input), *decompressed)
	}
}
