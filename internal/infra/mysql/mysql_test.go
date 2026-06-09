package mysql

import (
	"testing"

	"github.com/my/app/internal/shared/config"
)

func TestOpenDisabledReturnsNil(t *testing.T) {
	db, err := Open(config.MySQL{Enabled: false})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if db != nil {
		t.Fatal("Open() db is not nil")
	}
}

func TestOpenEnabledRequiresDSN(t *testing.T) {
	_, err := Open(config.MySQL{Enabled: true})
	if err == nil {
		t.Fatal("Open() error = nil, want error")
	}
}

func TestOpenRejectsAddressWithoutNetworkWrapper(t *testing.T) {
	_, err := Open(config.MySQL{Enabled: true, DSN: "user:pass@localhost:3306/tenancy"})
	if err == nil {
		t.Fatal("Open() error = nil, want error")
	}
	if err.Error() != "mysql dsn must use tcp" {
		t.Fatalf("Open() error = %q", err.Error())
	}
}
