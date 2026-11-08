package db

import (
	"testing"
	"time"
)

func TestEnvDuration(t *testing.T) {
	const name = "MONGODB_TEST_TIMEOUT"

	t.Setenv(name, "")
	if d, err := envDuration(name, 7*time.Second); err != nil || d != 7*time.Second {
		t.Errorf("unset: got %v, %v; want 7s, nil", d, err)
	}

	t.Setenv(name, "1500ms")
	if d, err := envDuration(name, time.Second); err != nil || d != 1500*time.Millisecond {
		t.Errorf("1500ms: got %v, %v", d, err)
	}

	t.Setenv(name, "0")
	if d, err := envDuration(name, time.Second); err != nil || d != 0 {
		t.Errorf("0: got %v, %v; want 0, nil", d, err)
	}

	for _, bad := range []string{"5", "abc", "-1s"} {
		t.Setenv(name, bad)
		if _, err := envDuration(name, time.Second); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestLoadTimeoutConfigDefaults(t *testing.T) {
	for _, name := range []string{
		"MONGODB_SERVER_SELECTION_TIMEOUT", "MONGODB_CONNECT_TIMEOUT", "MONGODB_SOCKET_TIMEOUT",
		"MONGODB_HEALTH_TIMEOUT", "MONGODB_STARTUP_TIMEOUT",
	} {
		t.Setenv(name, "")
	}
	cfg, err := loadTimeoutConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := timeoutConfig{5 * time.Second, 5 * time.Second, 15 * time.Second, 2 * time.Second, 30 * time.Second}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}

	t.Setenv("MONGODB_SOCKET_TIMEOUT", "bogus")
	if _, err := loadTimeoutConfig(); err == nil {
		t.Error("expected an error for an invalid MONGODB_SOCKET_TIMEOUT")
	}
}

func TestZeroRejectedExceptSocket(t *testing.T) {
	for _, name := range []string{"MONGODB_SERVER_SELECTION_TIMEOUT", "MONGODB_CONNECT_TIMEOUT", "MONGODB_HEALTH_TIMEOUT", "MONGODB_STARTUP_TIMEOUT"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "0")
			if _, err := loadTimeoutConfig(); err == nil {
				t.Errorf("%s=0: expected an error", name)
			}
		})
	}
	t.Setenv("MONGODB_SOCKET_TIMEOUT", "0")
	if cfg, err := loadTimeoutConfig(); err != nil || cfg.Socket != 0 {
		t.Errorf("MONGODB_SOCKET_TIMEOUT=0: got %v, %v; want 0, nil", cfg.Socket, err)
	}
}
