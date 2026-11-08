package db

import (
	"fmt"
	"os"
	"time"
)

// Default MongoDB timeouts. Each one can be overridden with the env var named next to it (a Go
// duration such as "5s" or "1500ms"), and a value given in MONGODB_URI (e.g. serverSelectionTimeoutMS)
// takes precedence over both. See DEPLOYMENT.md.
const (
	// MONGODB_SERVER_SELECTION_TIMEOUT. The driver default is 30s, which made every request hang for
	// 30s when Mongo was down (issue #298). Mongo runs on the same host / Docker network, so selection
	// normally takes milliseconds; 5s matches the mongo container healthcheck timeout.
	defaultServerSelectionTimeout = 5 * time.Second

	// MONGODB_CONNECT_TIMEOUT. Bounds the TCP dial and the heartbeat connection setup (driver default
	// 30s). With MongoDB 4.4+ the driver uses streaming heartbeats, so a mongod that accepts connections
	// but stops replying is marked unreachable after about connect timeout + 10s heartbeat = 15s.
	defaultConnectTimeout = 5 * time.Second

	// MONGODB_SOCKET_TIMEOUT. Bounds every single read/write on a connection (including cursor
	// getMore) when Mongo accepts the connection but never replies. 15s leaves headroom for the
	// whole-collection analytics and bot aggregations. 0 disables it (driver default).
	defaultSocketTimeout = 15 * time.Second

	// MONGODB_HEALTH_TIMEOUT. Used by /api/health and by the recovery middleware's ping. Well inside
	// the compose healthcheck timeout of 10s.
	defaultHealthTimeout = 2 * time.Second

	// MONGODB_STARTUP_TIMEOUT. How long Init keeps pinging Mongo before it logs an error and starts
	// in degraded mode (about 6 attempts with the default server selection timeout).
	defaultStartupTimeout = 30 * time.Second
)

// HealthTimeout is the deadline for health pings (/api/health and the recovery middleware).
// It is set from MONGODB_HEALTH_TIMEOUT by Init.
var HealthTimeout = defaultHealthTimeout

type timeoutConfig struct {
	ServerSelection time.Duration
	Connect         time.Duration
	Socket          time.Duration
	Health          time.Duration
	Startup         time.Duration
}

// Reads a duration from the env var `name`, returning `def` if it is unset or empty
func envDuration(name string, def time.Duration) (time.Duration, error) {
	return envDurationMin(name, def, 0)
}

// Same as envDuration, but also rejects values below `min`. The server selection, connect, health and
// startup timeouts use min = 1ns: 0 would turn the server selection and connect timeouts off (requests
// hang again as in issue #298) and would make every health ping fail immediately.
func envDurationMin(name string, def time.Duration, min time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return def, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, value, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid %s %q: must not be negative", name, value)
	}
	if d < min {
		return 0, fmt.Errorf("invalid %s %q: must be greater than 0", name, value)
	}
	return d, nil
}

// Reads all MongoDB timeouts from the environment
func loadTimeoutConfig() (timeoutConfig, error) {
	var cfg timeoutConfig
	var err error
	if cfg.ServerSelection, err = envDurationMin("MONGODB_SERVER_SELECTION_TIMEOUT", defaultServerSelectionTimeout, time.Nanosecond); err != nil {
		return cfg, err
	}
	if cfg.Connect, err = envDurationMin("MONGODB_CONNECT_TIMEOUT", defaultConnectTimeout, time.Nanosecond); err != nil {
		return cfg, err
	}
	if cfg.Socket, err = envDuration("MONGODB_SOCKET_TIMEOUT", defaultSocketTimeout); err != nil {
		return cfg, err
	}
	if cfg.Health, err = envDurationMin("MONGODB_HEALTH_TIMEOUT", defaultHealthTimeout, time.Nanosecond); err != nil {
		return cfg, err
	}
	if cfg.Startup, err = envDurationMin("MONGODB_STARTUP_TIMEOUT", defaultStartupTimeout, time.Nanosecond); err != nil {
		return cfg, err
	}
	return cfg, nil
}
