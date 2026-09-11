package chat

import (
	"testing"

	"github.com/my/app/internal/shared/ctxkey"
)

func TestAuthorizeChatDebugRequiresTheRequest(t *testing.T) {
	t.Setenv("APP_ENV", "local")

	p := ctxkey.Principal{Perms: []string{chatDebugPermission}}

	if authorizeChatDebug(p, false) {
		t.Fatal("debug must stay off when the request did not ask for it")
	}
}

func TestAuthorizeChatDebugGrantedByPermission(t *testing.T) {
	t.Setenv("APP_ENV", "local")

	p := ctxkey.Principal{Role: "tenant_admin", Perms: []string{chatDebugPermission}}

	if !authorizeChatDebug(p, true) {
		t.Fatal("a principal holding the permission should get debug outside production")
	}
}

func TestAuthorizeChatDebugDeniedInProduction(t *testing.T) {
	for _, env := range []string{"prod", "production", "PRODUCTION"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("APP_ENV", env)

			p := ctxkey.Principal{Role: "superadmin", Perms: []string{chatDebugPermission}}

			if authorizeChatDebug(p, true) {
				t.Fatalf("debug must be off in %s even for a permitted principal", env)
			}
		})
	}
}

func TestAuthorizeChatDebugNoRoleBypass(t *testing.T) {
	t.Setenv("APP_ENV", "local")

	for _, role := range []string{"system", "superadmin", "super_admin"} {
		t.Run(role, func(t *testing.T) {
			p := ctxkey.Principal{Role: role, Perms: []string{"ai:reply:create"}}

			if authorizeChatDebug(p, true) {
				t.Fatalf("role %q must not grant debug without the permission", role)
			}
		})
	}
}
