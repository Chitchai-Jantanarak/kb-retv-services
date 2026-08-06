package tools

import "testing"

func TestThresholdsFallBackToKindDefaults(t *testing.T) {
	cases := []struct {
		kind       string
		in         Thresholds
		wantAccept float64
		wantFloor  float64
	}{
		{"read", Thresholds{}, 0.55, 0.35},
		{"retrieval", Thresholds{}, 0.55, 0.35},
		{"write", Thresholds{}, 0.62, 0.35},
		{"read", Thresholds{Accept: 0.6}, 0.6, 0.35},
		{"write", Thresholds{Accept: 0.7, Floor: 0.4}, 0.7, 0.4},
		{"", Thresholds{}, 0.55, 0.35},
		{"write", Thresholds{Accept: 0.3, Floor: 0.4}, 0.3, 0.3},
	}
	for _, c := range cases {
		accept, floor := thresholdsFor(Tool{Kind: c.kind, Thresholds: c.in})
		if accept != c.wantAccept || floor != c.wantFloor {
			t.Errorf("kind=%q in=%+v got (%.2f, %.2f), want (%.2f, %.2f)",
				c.kind, c.in, accept, floor, c.wantAccept, c.wantFloor)
		}
	}
}

func TestWriteToolsDefaultToStricterAcceptThanReads(t *testing.T) {
	read, _ := thresholdsFor(Tool{Kind: "read"})
	write, _ := thresholdsFor(Tool{Kind: "write"})
	if write <= read {
		t.Fatalf("write accept %.2f must exceed read accept %.2f", write, read)
	}
}
