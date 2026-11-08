package middleware

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
	"go.mongodb.org/mongo-driver/mongo"
)

func init() {
	gin.SetMode(gin.TestMode)
	gin.DefaultErrorWriter = io.Discard
}

// Runs a request against a router whose only handler panics with `value`
func serveWithPanic(t *testing.T, value interface{}, ping func(context.Context) error) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.Use(DBAwareRecovery(ping, 100*time.Millisecond))
	router.GET("/", func(c *gin.Context) { panic(value) })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	return w
}

func assertServiceUnavailable(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "10" {
		t.Errorf("Retry-After = %q, want 10", got)
	}
	if body := strings.TrimSpace(w.Body.String()); body != `{"error":"service-unavailable"}` {
		t.Errorf("body = %s", body)
	}
}

func TestRecoveryUnavailableErrorSkipsPing(t *testing.T) {
	pinged := false
	ping := func(context.Context) error { pinged = true; return nil }

	w := serveWithPanic(t, mongo.ErrClientDisconnected, ping)
	assertServiceUnavailable(t, w)
	if pinged {
		t.Error("ping should not be called for a classified error")
	}
}

func TestRecoveryStringPanicWithFailingPing(t *testing.T) {
	ping := func(context.Context) error { return errors.New("unreachable") }
	assertServiceUnavailable(t, serveWithPanic(t, "boom", ping))
}

func TestRecoveryPingRespectsTimeout(t *testing.T) {
	ping := func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
	start := time.Now()
	assertServiceUnavailable(t, serveWithPanic(t, "boom", ping))
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v, want about 100ms", elapsed)
	}
}

func TestRecoveryStringPanicWithHealthyDB(t *testing.T) {
	ping := func(context.Context) error { return nil }
	w := serveWithPanic(t, "boom", ping)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", w.Body.String())
	}
}

func TestRecoveryOtherErrorWithHealthyDB(t *testing.T) {
	w := serveWithPanic(t, errors.New("x"), func(context.Context) error { return nil })
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestRecoveryNilPing(t *testing.T) {
	w := serveWithPanic(t, "boom", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestRecoveryAfterResponseWritten(t *testing.T) {
	router := gin.New()
	router.Use(DBAwareRecovery(nil, 100*time.Millisecond))
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		panic(mongo.ErrClientDisconnected)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); strings.Contains(body, "service-unavailable") {
		t.Errorf("body contains service-unavailable: %q", body)
	}
	if body := strings.TrimSpace(w.Body.String()); body != `{"status":"ok"}` {
		t.Errorf("body = %q, want original response", body)
	}
}
