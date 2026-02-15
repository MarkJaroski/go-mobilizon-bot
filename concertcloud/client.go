package concertcloud

import (
	"context"
	"net/http"

	"github.com/hashicorp/go-hclog"
)

// Client represents a ConcertCloud/event-api REST client
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     hclog.Logger
}

// Config holds client configuration
type Config struct {
	BaseURL    string
	Logger     hclog.Logger
	HTTPClient *http.Client
}

// NewClient creates a new ConcertCloud client
func NewClient(config Config) (*Client, error) {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.concertcloud.live"
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}

	return &Client{
		baseURL:    config.BaseURL,
		httpClient: config.HTTPClient,
		logger:     config.Logger,
	}, nil
}

// QueryParams holds parameters for event queries
type QueryParams struct {
	City    string
	Country string
	Limit   int
	Page    int
	Radius  int
	Date    string
}

// GetEvents fetches events from ConcertCloud API
func (c *Client) GetEvents(ctx context.Context, params QueryParams) (*EventResponse, error) {
	// Build query string
	// Make HTTP GET request
	// Parse response
}

// GetEventsByCity is a convenience method for city-based queries
func (c *Client) GetEventsByCity(ctx context.Context, city string, limit int) (*EventResponse, error) {
	return c.GetEvents(ctx, QueryParams{
		City:  city,
		Limit: limit,
	})
}

// GetEventsByCountry is a convenience method for country-based queries
func (c *Client) GetEventsByCountry(ctx context.Context, country string, limit int) (*EventResponse, error) {
	return c.GetEvents(ctx, QueryParams{
		Country: country,
		Limit:   limit,
	})
}
