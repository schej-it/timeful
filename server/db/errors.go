package db

import (
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/x/mongo/driver/auth"
	"go.mongodb.org/mongo-driver/x/mongo/driver/topology"
	"schej.it/server/logger"
)

// IsUnavailable reports whether err means MongoDB could not be reached (server selection timeout,
// driver network error including socket timeouts, or a disconnected client), as opposed to an error in
// the query itself. It only matches errors that come from the mongo driver: a plain net.Error timeout or
// context.DeadlineExceeded may come from another service (e.g. a Google API call), so the recovery
// middleware checks those with a ping instead.
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if mongo.IsNetworkError(err) {
		return true
	}
	var sse topology.ServerSelectionError
	if errors.As(err, &sse) {
		return true
	}
	return errors.Is(err, mongo.ErrClientDisconnected)
}

func isAuthError(err error) bool {
	var authErr *auth.Error
	return errors.As(err, &authErr)
}

// Logs err with the caller's file and line, then panics with err itself (not a string) so the
// recovery middleware can tell a database outage apart from other panics
func panicOnErr(err error) {
	logger.StdErr.Output(2, fmt.Sprintln(err))
	panic(err)
}
