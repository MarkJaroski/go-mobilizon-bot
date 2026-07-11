package concertcloud

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidParams indicates invalid query parameters
	ErrInvalidParams = errors.New("invalid query parameters")

	// ErrAPIError indicates an error from the ConcertCloud API
	ErrAPIError = errors.New("API error")

	// ErrNetworkError indicates a network-related error
	ErrNetworkError = errors.New("network error")
)

// APIError represents an error from the ConcertCloud API
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (status %d): %s", e.StatusCode, e.Message)
}
