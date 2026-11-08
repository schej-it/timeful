package routes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"schej.it/server/logger"
)

func newHealthRouter(ping func(context.Context) error, timeout time.Duration) *gin.Engine {
	gin.SetMode(gin.TestMode)
	if logger.StdErr == nil {
		logger.Init(io.Discard)
	}
	router := gin.New()
	router.Match([]string{http.MethodGet, http.MethodHead}, "/api/health", healthHandler(ping, timeout))
	return router
}

func TestHealth(t *testing.T) {
	cases := []struct {
		name     string
		ping     func(context.Context) error
		wantCode int
		wantBody string
	}{
		{"ok", func(context.Context) error { return nil }, http.StatusOK, `{"status":"ok"}`},
		{"unreachable", func(context.Context) error { return errors.New("server selection timeout: secret detail") }, http.StatusServiceUnavailable, `{"status":"unavailable","error":"service-unavailable"}`},
	}
	for _, tc := range cases {
		router := newHealthRouter(tc.ping, time.Second)
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(method, "/api/health", nil))
			if w.Code != tc.wantCode {
				t.Errorf("%s %s: status = %d, want %d", tc.name, method, w.Code, tc.wantCode)
			}
			if method == http.MethodGet && strings.TrimSpace(w.Body.String()) != tc.wantBody {
				t.Errorf("%s: body = %s, want %s", tc.name, w.Body.String(), tc.wantBody)
			}
		}
	}
}

func TestHealthUsesTimeout(t *testing.T) {
	router := newHealthRouter(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, 100*time.Millisecond)
	start := time.Now()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v, want about 100ms", elapsed)
	}
}
