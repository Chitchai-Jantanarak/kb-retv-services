package omnichannel

import "testing"

// The reference digest was produced by PHP's hash_hmac (the exact call in
// App\Support\EmailHash) with the fixed key below — no real secret in source.
// If this breaks, the Go and PHP algorithms have diverged and sender-email
// backfill will silently miss every customer.
func TestEmailHashMatchesPHPHmac(t *testing.T) {
	const key = "test-key-123"
	const want = "f4742356a3fbc8f5124037dfb25135a3107345c62972ffcb116189c69118ba07"
	if got := EmailHash("ops@acme.com", key); got != want {
		t.Fatalf("EmailHash mismatch\n got=%s\nwant=%s\n(Go and PHP EmailHash diverged)", got, want)
	}
}

func TestEmailHashNormalizesAndGuardsEmpty(t *testing.T) {
	const key = "test-key-123"
	if EmailHash("  OPS@Acme.com ", key) != EmailHash("ops@acme.com", key) {
		t.Fatal("EmailHash must lowercase and trim before hashing")
	}
	if EmailHash("   ", key) != "" {
		t.Fatal("blank email must hash to empty string")
	}
}
