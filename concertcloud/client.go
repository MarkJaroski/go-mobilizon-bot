// Package concertcloud is a Go client library for the concertcloud.live
// REST API
package concertcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/hashicorp/go-hclog"
)

const DefaultBaseURL = "https://api.concertcloud.live"

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
		config.BaseURL = DefaultBaseURL
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

// GetEvents fetches events from ConcertCloud API
func (c *Client) GetEvents(ctx context.Context, params QueryParams) (*EventResponse, error) {
	// Build query string
	query := url.Values{}

	if params.City != "" {
		query.Set("city", params.City)
	}
	if params.Country != "" {
		query.Set("country", params.Country)
	}
	if params.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", params.Limit))
	}
	if params.Page > 0 {
		query.Set("page", fmt.Sprintf("%d", params.Page))
	}
	if params.Radius > 0 {
		query.Set("radius", fmt.Sprintf("%d", params.Radius))
	}
	if params.Date != "" {
		query.Set("date", params.Date)
	}

	// buildURL
	apiURL := fmt.Sprintf("%s/api/events?%s", c.baseURL, query.Encode())

	if c.logger != nil {
		c.logger.Debug("Fetching events from ConcertCloud", "url", apiURL)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch events: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response EventResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if c.logger != nil {
		c.logger.Info("Fetched events", "count", len(response.Data), "total", response.Total)
	}

	return &response, nil

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
