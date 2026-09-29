package task

import (
	"errors"
	"fmt"

	"modelhub/internal/gateway"
)

// P6-1: channel failover for the media/drama task pipeline. Adapters report
// structured failures so the worker can tell an availability problem (worth
// retrying on the next channel) from a request/content problem (would fail
// identically everywhere).

// UpstreamHTTPError is a non-2xx HTTP response from an upstream channel.
type UpstreamHTTPError struct {
	Status int
	Body   []byte
}

func (e *UpstreamHTTPError) Error() string {
	return fmt.Sprintf("upstream returned HTTP %d: %s", e.Status, trimBody(e.Body))
}

// TransportError is a network-level failure talking to an upstream channel.
type TransportError struct{ Err error }

func (e *TransportError) Error() string {
	return fmt.Sprintf("upstream request: %v", e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

// taskFailureRetryable reports whether a failed task attempt is worth
// retrying on another channel. Async upstream task failures ("upstream task
// failed: ..."), timeouts, parse errors and payload problems are NOT
// retryable here: the same content would hit the same wall elsewhere (or the
// failure already consumed the attempt budget).
func taskFailureRetryable(err error) bool {
	var ue *UpstreamHTTPError
	if errors.As(err, &ue) {
		return gateway.UpstreamRetryable(ue.Status, false)
	}
	var te *TransportError
	if errors.As(err, &te) {
		return true
	}
	return false
}
