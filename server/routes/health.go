/* The /health route reports whether the server can reach MongoDB */
package routes

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"schej.it/server/db"
	"schej.it/server/errs"
	"schej.it/server/logger"
)

func InitHealth(router *gin.Engine) {
	router.Match([]string{http.MethodGet, http.MethodHead}, "/api/health", healthHandler(db.Ping, db.HealthTimeout))
}

type healthResponse struct {
	Status string `json:"status" example:"ok"`
	Error  string `json:"error,omitempty" example:"service-unavailable"`
}

// @Summary Reports whether the server can reach MongoDB
// @Description Pings MongoDB. Returns 200 when it responds, 503 when it does not. Also accepts HEAD.
// @Tags health
// @Produce json
// @Success 200 {object} healthResponse
// @Failure 503 {object} healthResponse
// @Router /health [get]
func healthHandler(ping func(context.Context) error, timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		if err := ping(ctx); err != nil {
			logger.StdErr.Printf("Health check failed, MongoDB unreachable: %v", err)
			c.JSON(http.StatusServiceUnavailable, healthResponse{Status: "unavailable", Error: errs.ServiceUnavailable})
			return
		}
		c.JSON(http.StatusOK, healthResponse{Status: "ok"})
	}
}
