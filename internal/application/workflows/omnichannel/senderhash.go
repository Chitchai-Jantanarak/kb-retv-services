package omnichannel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// EmailHash mirrors App\Support\EmailHash::from in the Laravel app exactly:
// hash_hmac('sha256', mb_strtolower(trim(email)), config('app.key')). The key
// is the literal APP_KEY string (the "base64:..." value, not decoded), so
// go-api and Laravel produce identical digests and sender lookups line up.
func EmailHash(email, appKey string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(appKey))
	mac.Write([]byte(normalized))
	return hex.EncodeToString(mac.Sum(nil))
}
