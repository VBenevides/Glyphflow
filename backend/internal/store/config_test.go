package store

import "testing"

func TestConfigNameAllowlist(t *testing.T) {
	for name := range allowedConfigNames {
		if err := validateConfigName(name); err != nil {
			t.Errorf("allowed config %q rejected: %v", name, err)
		}
	}
	for _, name := range []string{"DEFAULT_ROLE", "state.auth", "PASSWORD_PEPPER", "ACCESS_TOKEN_SECRET", "unknown"} {
		if err := validateConfigName(name); err == nil {
			t.Errorf("config key %q was accepted", name)
		}
	}
}
