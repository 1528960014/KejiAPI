package task

import (
	"errors"
	"fmt"
	"testing"
)

func TestTaskFailureRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"http 500", &UpstreamHTTPError{Status: 500, Body: []byte("boom")}, true},
		{"http 401 (channel key rejected)", &UpstreamHTTPError{Status: 401}, true},
		{"http 429", &UpstreamHTTPError{Status: 429}, true},
		{"http 400 (request problem)", &UpstreamHTTPError{Status: 400}, false},
		{"http 404", &UpstreamHTTPError{Status: 404}, false},
		{"transport", &TransportError{Err: errors.New("dial refused")}, true},
		{"async task failed (content problem)", errors.New("upstream task failed: content policy"), false},
		{"plain error", errors.New("whatever"), false},
		{"wrapped retryable", fmt.Errorf("outer: %w", &UpstreamHTTPError{Status: 503}), true},
		{"wrapped non-retryable", fmt.Errorf("outer: %w", &UpstreamHTTPError{Status: 400}), false},
		{"transport wrapped", fmt.Errorf("outer: %w", &TransportError{Err: errors.New("timeout")}), true},
	}
	for _, tc := range cases {
		if got := taskFailureRetryable(tc.err); got != tc.want {
			t.Errorf("%s: taskFailureRetryable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUpstreamHTTPErrorMessage(t *testing.T) {
	// The user-facing text must stay the same as before P6-1.
	e := &UpstreamHTTPError{Status: 401, Body: []byte("invalid key")}
	if got, want := e.Error(), "upstream returned HTTP 401: invalid key"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	e2 := &TransportError{Err: errors.New("dial tcp refused")}
	if got, want := e2.Error(), "upstream request: dial tcp refused"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
