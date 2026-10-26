package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestRBACShadow(t *testing.T) {
	t.Setenv("RBAC_MODE", "")

	for _, tc := range []struct {
		name     string
		rbacMode string
		want     bool
	}{
		{name: "empty defaults to enforce", rbacMode: "", want: false},
		{name: "explicit enforce", rbacMode: "enforce", want: false},
		{name: "shadow from config", rbacMode: "shadow", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (AppConfig{RbacMode: tc.rbacMode}).RBACShadow(); got != tc.want {
				t.Fatalf("RBACShadow() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRBACShadowEnvOverridesConfig(t *testing.T) {
	t.Setenv("RBAC_MODE", "enforce")
	if (AppConfig{RbacMode: "shadow"}).RBACShadow() {
		t.Fatal("RBAC_MODE=enforce must override rbac_mode=shadow")
	}

	t.Setenv("RBAC_MODE", "shadow")
	if !(AppConfig{RbacMode: "enforce"}).RBACShadow() {
		t.Fatal("RBAC_MODE=shadow must override rbac_mode=enforce")
	}
}

func TestRBACShadowUnknownModePanics(t *testing.T) {
	t.Setenv("RBAC_MODE", "")

	defer func() {
		got := recover()
		msg, ok := got.(string)
		if !ok || !strings.Contains(msg, `unknown rbac_mode "staging"`) {
			t.Fatalf("panic = %v, want unknown rbac_mode message", got)
		}
	}()
	(AppConfig{RbacMode: "staging"}).RBACShadow()
}

func TestRbacModeTomlTag(t *testing.T) {
	// The toml tag must stay "rbac_mode": services set it in config/*.toml.
	var decoded struct {
		App AppConfig `toml:"app"`
	}
	if _, err := toml.Decode("[app]\nrbac_mode = \"shadow\"\n", &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.App.RbacMode != "shadow" {
		t.Fatalf("RbacMode = %q, want shadow", decoded.App.RbacMode)
	}
}
