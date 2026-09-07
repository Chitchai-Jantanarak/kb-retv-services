package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/shared/providercrypto"
)

func TestAgentLookupRejectsBadCompanyID(t *testing.T) {
	l := NewAgentLookup(nil, "")
	cases := []struct {
		name      string
		companyID int64
	}{
		{name: "zero", companyID: 0},
		{name: "negative", companyID: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := l.AgentFor(context.Background(), tc.companyID)
			if err == nil {
				t.Fatal("err = nil, want company_id error")
			}
			if !strings.Contains(err.Error(), "company_id must be positive") {
				t.Fatalf("err = %q", err.Error())
			}
		})
	}
}

func testProviderKey() []byte {
	return []byte("01234567890123456789012345678901")
}

func TestApplyProviderConfigFillsBackupAndFailover(t *testing.T) {
	key := testProviderKey()
	encKey, err := providercrypto.Encrypt("backup-secret", key)
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	l := &AgentLookup{providerKey: key}

	raw := `{"base_url":"https://primary.example","backup_vendor":"anthropic","backup_model":"claude-haiku-4-5","backup_api_key_enc":"` + encKey + `","backup_base_url":"https://backup.example","failover":"Quota"}`

	var cfg llm.AgentConfig
	l.applyProviderConfig(&cfg, raw)

	if cfg.BaseURL != "https://primary.example" {
		t.Fatalf("cfg.BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Backup == nil {
		t.Fatal("cfg.Backup = nil, want populated backup config")
	}
	if cfg.Backup.Vendor != "anthropic" || cfg.Backup.Model != "claude-haiku-4-5" {
		t.Fatalf("cfg.Backup = %+v", cfg.Backup)
	}
	if cfg.Backup.BaseURL != "https://backup.example" {
		t.Fatalf("cfg.Backup.BaseURL = %q", cfg.Backup.BaseURL)
	}
	if cfg.Backup.APIKey != "backup-secret" {
		t.Fatalf("cfg.Backup.APIKey = %q, want decrypted backup-secret", cfg.Backup.APIKey)
	}
	if cfg.Failover != "quota" {
		t.Fatalf("cfg.Failover = %q, want quota", cfg.Failover)
	}
}

func TestApplyProviderConfigNoBackupLeavesNilAndFailoverOff(t *testing.T) {
	l := &AgentLookup{}
	var cfg llm.AgentConfig
	l.applyProviderConfig(&cfg, `{"base_url":"https://primary.example"}`)

	if cfg.Backup != nil {
		t.Fatalf("cfg.Backup = %+v, want nil", cfg.Backup)
	}
	if cfg.Failover != "off" {
		t.Fatalf("cfg.Failover = %q, want off", cfg.Failover)
	}
}

func TestApplyProviderConfigInvalidFailoverNormalizesToOff(t *testing.T) {
	l := &AgentLookup{}
	var cfg llm.AgentConfig
	l.applyProviderConfig(&cfg, `{"backup_vendor":"gemini","failover":"bogus"}`)

	if cfg.Failover != "off" {
		t.Fatalf("cfg.Failover = %q, want off for invalid value", cfg.Failover)
	}
	if cfg.Backup == nil || cfg.Backup.Vendor != "gemini" {
		t.Fatalf("cfg.Backup = %+v, want vendor gemini", cfg.Backup)
	}
}
