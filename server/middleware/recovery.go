package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"schej.it/server/db"
	"schej.it/server/errs"
	"schej.it/server/responses"
)

// Seconds a client should wait before retrying after a 503
const serviceUnavailableRetryAfter = "10"

// DBAwareRecovery replaces gin.Recovery. It keeps gin's panic logging and broken pipe handling, but
// returns 503 Service Unavailable instead of an empty 500 when the panic was caused by MongoDB being
// unreachable (issue #298):
//  1. The panic value is an error that db.IsUnavailable classifies as an outage: 503, no ping.
//  2. Otherwise (e.g. a string from logger.StdErr.Panicln in a route handler) ping mongo within
//     `timeout`; if the ping fails: 503.
//  3. Otherwise: empty 500, as gin.Recovery does.
//
// `ping` is passed in so tests do not need a running mongo. A nil ping skips step 2.
func DBAwareRecovery(ping func(context.Context) error, timeout time.Duration) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		if c.Writer.Written() {
			c.Abort()
			return
		}
		if isDBOutage(c, recovered, ping, timeout) {
			c.Header("Retry-After", serviceUnavailableRetryAfter)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, responses.Error{Error: errs.ServiceUnavailable})
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

func isDBOutage(c *gin.Context, recovered interface{}, ping func(context.Context) error, timeout time.Duration) bool {
	if err, ok := recovered.(error); ok && db.IsUnavailable(err) {
		return true
	}
	if ping == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return ping(ctx) != nil
}
