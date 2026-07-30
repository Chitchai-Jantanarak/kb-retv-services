package htmltext

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		name           string
		input          string
		wantUnchanged  bool
		mustNotContain string
	}{
		{
			name:           "leaked login credentials",
			input:          `<p>Wrong logo to login</p><p>Admin user/password unable to login</p><p>user: h311130</p><p>password: yek10123</p>`,
			mustNotContain: "yek10123",
		},
		{
			name:           "api key",
			input:          "api_key: sk-abc123",
			mustNotContain: "sk-abc123",
		},
		{
			name:           "thai password label",
			input:          "รหัสผ่าน: p@ssw0rd",
			mustNotContain: "p@ssw0rd",
		},
		{
			name:          "benign text unchanged",
			input:         "The user reported a login problem",
			wantUnchanged: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactSecrets(tc.input)
			if tc.wantUnchanged {
				if got != tc.input {
					t.Fatalf("got %q, want unchanged %q", got, tc.input)
				}
				return
			}
			if strings.Contains(got, tc.mustNotContain) {
				t.Fatalf("secret leaked: got %q, must not contain %q", got, tc.mustNotContain)
			}
		})
	}
}
