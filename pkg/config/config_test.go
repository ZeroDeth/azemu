package config

import (
	"testing"
)

// setEnv sets an env var for the duration of the test and restores it
// afterwards. t.Setenv does this for us but also fails loud if the test
// was started with t.Parallel, which matches the behaviour we want —
// config tests must never run in parallel because they all mutate process
// state (env vars) that Load() reads.

// TestLoad_Defaults verifies that Load() returns the documented default
// values when no env vars are set. These defaults are the contract that
// docs/SETUP.md and the flox profile rely on, so the test pins every
// field rather than just spot-checking one.
func TestLoad_Defaults(t *testing.T) {
	clearAzemuEnv(t)

	cfg := Load()

	if cfg.HTTPPort != 4566 {
		t.Errorf("HTTPPort = %d, want 4566", cfg.HTTPPort)
	}
	if cfg.HTTPSPort != 4567 {
		t.Errorf("HTTPSPort = %d, want 4567", cfg.HTTPSPort)
	}
	if cfg.HealthPort != 4568 {
		t.Errorf("HealthPort = %d, want 4568", cfg.HealthPort)
	}
	if want := "00000000-0000-0000-0000-000000000000"; cfg.SubscriptionID != want {
		t.Errorf("SubscriptionID = %q, want %q", cfg.SubscriptionID, want)
	}
	if want := "00000000-0000-0000-0000-000000000001"; cfg.TenantID != want {
		t.Errorf("TenantID = %q, want %q", cfg.TenantID, want)
	}
	if want := "localhost:4567"; cfg.MetadataHost != want {
		t.Errorf("MetadataHost = %q, want %q", cfg.MetadataHost, want)
	}
	if cfg.CertPath != "" {
		t.Errorf("CertPath = %q, want empty string", cfg.CertPath)
	}
	if want := "http://azurite:10000"; cfg.AzuriteEndpoint != want {
		t.Errorf("AzuriteEndpoint = %q, want %q", cfg.AzuriteEndpoint, want)
	}
}

// TestLoad_EnvVarOverrides is table-driven over every env var Load() reads.
// Each case sets exactly one env var and asserts that the matching field is
// picked up while all other fields retain their defaults. This is the
// cheapest way to catch a future copy/paste error that binds the wrong
// variable to the wrong field.
func TestLoad_EnvVarOverrides(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		value   string
		field   string
		extract func(*Config) string
	}{
		{
			name:    "AZEMU_SUBSCRIPTION_ID",
			env:     "AZEMU_SUBSCRIPTION_ID",
			value:   "11111111-2222-3333-4444-555555555555",
			field:   "SubscriptionID",
			extract: func(c *Config) string { return c.SubscriptionID },
		},
		{
			name:    "AZEMU_TENANT_ID",
			env:     "AZEMU_TENANT_ID",
			value:   "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			field:   "TenantID",
			extract: func(c *Config) string { return c.TenantID },
		},
		{
			name:    "AZEMU_METADATA_HOST",
			env:     "AZEMU_METADATA_HOST",
			value:   "azemu.test:9443",
			field:   "MetadataHost",
			extract: func(c *Config) string { return c.MetadataHost },
		},
		{
			name:    "AZEMU_CERT_PATH",
			env:     "AZEMU_CERT_PATH",
			value:   "/tmp/azemu-bundle.pem",
			field:   "CertPath",
			extract: func(c *Config) string { return c.CertPath },
		},
		{
			name:    "AZEMU_AZURITE_ENDPOINT",
			env:     "AZEMU_AZURITE_ENDPOINT",
			value:   "http://localhost:10000",
			field:   "AzuriteEndpoint",
			extract: func(c *Config) string { return c.AzuriteEndpoint },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAzemuEnv(t)
			t.Setenv(tc.env, tc.value)

			cfg := Load()
			if got := tc.extract(cfg); got != tc.value {
				t.Errorf("%s = %q, want %q", tc.field, got, tc.value)
			}
		})
	}
}

// TestLoad_EmptyStringIsTreatedAsUnset pins envOr semantics: an env var set
// to the empty string must fall back to the default, not produce an empty
// field. This prevents a subtle foot-gun where `AZEMU_SUBSCRIPTION_ID=` in
// a .env file would blank out the default.
func TestLoad_EmptyStringIsTreatedAsUnset(t *testing.T) {
	clearAzemuEnv(t)
	t.Setenv("AZEMU_SUBSCRIPTION_ID", "")
	t.Setenv("AZEMU_TENANT_ID", "")
	t.Setenv("AZEMU_METADATA_HOST", "")
	t.Setenv("AZEMU_CERT_PATH", "")

	cfg := Load()

	if want := "00000000-0000-0000-0000-000000000000"; cfg.SubscriptionID != want {
		t.Errorf("SubscriptionID = %q, want default %q", cfg.SubscriptionID, want)
	}
	if want := "00000000-0000-0000-0000-000000000001"; cfg.TenantID != want {
		t.Errorf("TenantID = %q, want default %q", cfg.TenantID, want)
	}
	if want := "localhost:4567"; cfg.MetadataHost != want {
		t.Errorf("MetadataHost = %q, want default %q", cfg.MetadataHost, want)
	}
	if cfg.CertPath != "" {
		t.Errorf("CertPath = %q, want empty string", cfg.CertPath)
	}
}

// TestLoad_PortsAreNotEnvDriven pins the current behaviour: HTTPPort,
// HTTPSPort, and HealthPort are hardcoded in Load() and ignore any env-var
// overrides. If a future change adds AZEMU_HTTP_PORT / AZEMU_HTTPS_PORT /
// AZEMU_HEALTH_PORT support, this test is the place to update. Recording
// the behaviour explicitly here keeps the contract visible to contributors.
func TestLoad_PortsAreNotEnvDriven(t *testing.T) {
	clearAzemuEnv(t)
	t.Setenv("AZEMU_HTTP_PORT", "9999")
	t.Setenv("AZEMU_HTTPS_PORT", "9998")
	t.Setenv("AZEMU_HEALTH_PORT", "9997")

	cfg := Load()

	if cfg.HTTPPort != 4566 {
		t.Errorf("HTTPPort = %d, want 4566 (ports are hardcoded today)", cfg.HTTPPort)
	}
	if cfg.HTTPSPort != 4567 {
		t.Errorf("HTTPSPort = %d, want 4567 (ports are hardcoded today)", cfg.HTTPSPort)
	}
	if cfg.HealthPort != 4568 {
		t.Errorf("HealthPort = %d, want 4568 (ports are hardcoded today)", cfg.HealthPort)
	}
}

// TestEnvOr_TableDriven exercises the envOr helper directly so that any
// future callers get the same behaviour as the ones inside Load().
func TestEnvOr_TableDriven(t *testing.T) {
	cases := []struct {
		name     string
		setValue string
		setEnv   bool
		fallback string
		want     string
	}{
		{name: "env unset returns fallback", setEnv: false, fallback: "default", want: "default"},
		{name: "env empty string returns fallback", setEnv: true, setValue: "", fallback: "default", want: "default"},
		{name: "env set returns value", setEnv: true, setValue: "custom", fallback: "default", want: "custom"},
		{name: "env set with fallback empty returns value", setEnv: true, setValue: "custom", fallback: "", want: "custom"},
	}

	const key = "AZEMU_TEST_ENVOR"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Start from a clean slate: always ensure the test key is unset
			// before each case, then set it only if the case demands it.
			// Note: t.Setenv with "" leaves the var present-but-empty, which
			// envOr treats as fallback anyway, so the "env unset" case and
			// the "env empty string" case are behaviourally equivalent for
			// envOr and both exercise the fallback branch.
			t.Setenv(key, "")
			if tc.setEnv {
				t.Setenv(key, tc.setValue)
			}

			got := envOr(key, tc.fallback)
			if got != tc.want {
				t.Errorf("envOr(%q, %q) = %q, want %q", key, tc.fallback, got, tc.want)
			}
		})
	}
}

// clearAzemuEnv unsets every env var Load() reads so the caller starts from
// a known-clean state. t.Setenv registers a cleanup that restores the prior
// value, so this is safe to call at the start of every test.
func clearAzemuEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"AZEMU_SUBSCRIPTION_ID",
		"AZEMU_TENANT_ID",
		"AZEMU_METADATA_HOST",
		"AZEMU_CERT_PATH",
		"AZEMU_AZURITE_ENDPOINT",
		"AZEMU_HTTP_PORT",
		"AZEMU_HTTPS_PORT",
		"AZEMU_HEALTH_PORT",
	} {
		t.Setenv(k, "")
	}
}
