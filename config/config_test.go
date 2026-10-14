package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func TestMainConfigDecodeUnion(t *testing.T) {
	const input = `
[app]
app_key = "01234567890123456789012345678901"
otp_issuer = "E-IPO"
validate_over_ksei = true
mfa_user_types = [0, 1]

[database]
type = "postgres"
host = "localhost"

[sftp]
host = "sftp.example.test"

[recaptcha]
enabled = true
`
	var cfg MainConfig
	if _, err := toml.Decode(input, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.App.OtpIssuer != "E-IPO" || !cfg.App.ValidateOverKsei {
		t.Fatalf("unexpected app config: %+v", cfg.App)
	}
	if len(cfg.App.MFAUserTypes) != 2 {
		t.Fatalf("MFAUserTypes length = %d, want 2", len(cfg.App.MFAUserTypes))
	}
	if cfg.Database.Type != DBTypePostgres {
		t.Fatalf("database type = %q, want %q", cfg.Database.Type, DBTypePostgres)
	}
	if cfg.Sftp.Host != "sftp.example.test" || !cfg.Recaptcha.Enabled {
		t.Fatalf("optional sections not decoded: %+v %+v", cfg.Sftp, cfg.Recaptcha)
	}
}

func TestAppConfigMaxUploadBytesDefaultsTo50MB(t *testing.T) {
	var cfg AppConfig
	if got := cfg.MaxUploadBytes(); got != 50*1024*1024 {
		t.Fatalf("MaxUploadBytes() = %d, want 50MB", got)
	}
}

func TestAppConfigMaxUploadBytesUsesConfiguredValue(t *testing.T) {
	cfg := AppConfig{MaxUploadSizeMB: 5}
	if got := cfg.MaxUploadBytes(); got != 5*1024*1024 {
		t.Fatalf("MaxUploadBytes() = %d, want 5MB", got)
	}
}

func TestMainConfigMissingOptionalSectionsUseZeroValues(t *testing.T) {
	var cfg MainConfig
	if _, err := toml.Decode("[app]\notp_issuer='E-IPO'", &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Sftp.Host != "" || cfg.Recaptcha.Enabled || len(cfg.App.MFAUserTypes) != 0 {
		t.Fatalf("optional fields should use zero values: %+v", cfg)
	}
}
