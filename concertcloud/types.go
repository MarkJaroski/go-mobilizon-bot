package concertcloud

import (
	"context"

	"github.com/jakopako/event-api/models"
)

// EventFetcher defines the interface for fetching events
type EventFetcher interface {
	GetEvents(ctx context.Context, params QueryParams) (*EventResponse, error)
	GetEventsByCity(ctx context.Context, city string, limit int) (*EventResponse, error)
	GetEventsByCountry(ctx context.Context, country string, limit int) (*EventResponse, error)
}

// Event is an alias for models.Event from event-api
// This allows us to add methods or extend the type if needed
type Event = models.Event

// EventResponse represents the API response for event queries
type EventResponse struct {
	Data     []Event `json:"data"`
	Page     int     `json:"page"`
	Limit    int     `json:"limit"`
	Total    int     `json:"total"`
	LastPage int     `json:"last_page"`
}

// QueryParams holds parameters for event API queries
type QueryParams struct {
	City    string
	Country string
	Limit   int
	Page    int
	Radius  int
	Date    string
}
