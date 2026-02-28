package concertcloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestEventResponseParsing tests that various event response formats are parsed correctly
func TestEventResponseParsing(t *testing.T) {
	tests := []struct {
		name     string
		response string
		validate func(t *testing.T, resp *EventResponse)
		wantErr  bool
	}{
		{
			name: "minimal event data",
			response: `{
				"data": [{
					"title": "Minimal Event",
					"location": "Venue",
					"city": "City",
					"date": "2024-12-31T20:00:00Z",
					"url": "https://example.com/event",
					"type": "concert",
					"sourceUrl": "https://example.com"
				}],
				"page": 1,
				"limit": 10,
				"total": 1,
				"last_page": 1
			}`,
			validate: func(t *testing.T, resp *EventResponse) {
				if len(resp.Data) != 1 {
					t.Errorf("Expected 1 event, got %d", len(resp.Data))
				}
				if resp.Data[0].Title != "Minimal Event" {
					t.Errorf("Expected title 'Minimal Event', got '%s'", resp.Data[0].Title)
				}
			},
			wantErr: false,
		},
		{
			name: "event with all optional fields",
			response: `{
				"data": [{
					"title": "Complete Event",
					"location": "Main Venue",
					"city": "Zurich",
					"country": "Switzerland",
					"date": "2024-12-31T20:00:00Z",
					"offset": 60,
					"url": "https://example.com/event",
					"imageUrl": "https://example.com/image.jpg",
					"comment": "Detailed comment about the event",
					"type": "concert",
					"sourceUrl": "https://example.com",
					"genres": ["Rock", "Alternative"],
					"genresText": "Rock and alternative music",
					"address": {
						"locality": "Zurich",
						"postCode": "8001",
						"street": "Main Street",
						"houseNumber": "42",
						"country": "Switzerland",
						"state": "Zurich",
						"geolocation": {
							"type": "Point",
							"coordinates": [8.5417, 47.3769]
						}
					}
				}],
				"page": 1,
				"limit": 10,
				"total": 1,
				"last_page": 1
			}`,
			validate: func(t *testing.T, resp *EventResponse) {
				if len(resp.Data) != 1 {
					t.Fatalf("Expected 1 event, got %d", len(resp.Data))
				}
				event := resp.Data[0]
				if event.Country != "Switzerland" {
					t.Errorf("Expected country 'Switzerland', got '%s'", event.Country)
				}
				if event.ImageURL != "https://example.com/image.jpg" {
					t.Errorf("Expected imageUrl, got '%s'", event.ImageURL)
				}
				if len(event.Genres) != 2 {
					t.Errorf("Expected 2 genres, got %d", len(event.Genres))
				}
				if event.Address.PostCode != "8001" {
					t.Errorf("Expected postcode 8001, got '%s'", event.Address.PostCode)
				}
			},
			wantErr: false,
		},
		{
			name: "event with missing required fields",
			response: `{
				"data": [{
					"title": "Incomplete Event"
				}],
				"page": 1,
				"limit": 10,
				"total": 1,
				"last_page": 1
			}`,
			validate: func(t *testing.T, resp *EventResponse) {
				// Should still parse, but fields will be empty
				if len(resp.Data) != 1 {
					t.Errorf("Expected 1 event, got %d", len(resp.Data))
				}
			},
			wantErr: false,
		},
		{
			name: "multiple pages of events",
			response: `{
				"data": [
					{"title": "Event 1", "location": "V1", "city": "C1", "date": "2024-12-31T20:00:00Z", "url": "https://e1.com", "type": "concert", "sourceUrl": "https://s1.com"},
					{"title": "Event 2", "location": "V2", "city": "C2", "date": "2024-12-31T21:00:00Z", "url": "https://e2.com", "type": "concert", "sourceUrl": "https://s2.com"}
				],
				"page": 2,
				"limit": 2,
				"total": 10,
				"last_page": 5
			}`,
			validate: func(t *testing.T, resp *EventResponse) {
				if len(resp.Data) != 2 {
					t.Errorf("Expected 2 events, got %d", len(resp.Data))
				}
				if resp.Page != 2 {
					t.Errorf("Expected page 2, got %d", resp.Page)
				}
				if resp.LastPage != 5 {
					t.Errorf("Expected last_page 5, got %d", resp.LastPage)
				}
			},
			wantErr: false,
		},
		{
			name: "unicode and special characters in event data",
			response: `{
				"data": [{
					"title": "Fête de la Musique 2024 🎵",
					"location": "Zürich Hauptbahnhof",
					"city": "Zürich",
					"country": "Schweiz",
					"date": "2024-06-21T18:00:00Z",
					"url": "https://example.com/fête",
					"comment": "Un événement spécial avec des caractères spéciaux: é, è, ê, à, ç",
					"type": "festival",
					"sourceUrl": "https://example.com"
				}],
				"page": 1,
				"limit": 10,
				"total": 1,
				"last_page": 1
			}`,
			validate: func(t *testing.T, resp *EventResponse) {
				if len(resp.Data) != 1 {
					t.Fatalf("Expected 1 event, got %d", len(resp.Data))
				}
				event := resp.Data[0]
				if event.Title != "Fête de la Musique 2024 🎵" {
					t.Errorf("Unicode title not preserved: got '%s'", event.Title)
				}
				if event.City != "Zürich" {
					t.Errorf("Unicode city not preserved: got '%s'", event.City)
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client, _ := NewClient(Config{BaseURL: server.URL})
			ctx := context.Background()

			resp, err := client.GetEvents(ctx, QueryParams{City: "Test"})

			if (err != nil) != tt.wantErr {
				t.Errorf("GetEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && tt.validate != nil {
				tt.validate(t, resp)
			}
		})
	}
}

// TestQueryParamsEdgeCases tests edge cases in query parameter building
func TestQueryParamsEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		params     QueryParams
		checkQuery func(t *testing.T, query string)
	}{
		{
			name:   "empty params",
			params: QueryParams{},
			checkQuery: func(t *testing.T, query string) {
				// Should only have the base path
				if query != "/api/events?" && query != "/api/events" {
					t.Errorf("Expected empty or minimal query, got %s", query)
				}
			},
		},
		{
			name: "only optional parameters",
			params: QueryParams{
				Radius: 100,
				Date:   "2024-12-31",
			},
			checkQuery: func(t *testing.T, query string) {
				if !contains(query, "radius=100") {
					t.Error("Expected radius parameter")
				}
				if !contains(query, "date=2024-12-31") {
					t.Error("Expected date parameter")
				}
			},
		},
		{
			name: "zero values should not be sent",
			params: QueryParams{
				City:   "Lausanne",
				Limit:  0, // Zero should not be sent
				Page:   0, // Zero should not be sent
				Radius: 0, // Zero should not be sent
			},
			checkQuery: func(t *testing.T, query string) {
				if contains(query, "limit=0") {
					t.Error("Should not send limit=0")
				}
				if contains(query, "page=0") {
					t.Error("Should not send page=0")
				}
				if contains(query, "radius=0") {
					t.Error("Should not send radius=0")
				}
			},
		},
		{
			name: "special characters in city",
			params: QueryParams{
				City: "São Paulo",
			},
			checkQuery: func(t *testing.T, query string) {
				// URL encoding should handle special characters
				if !contains(query, "city=") {
					t.Error("Expected city parameter")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestQuery string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestQuery = r.URL.RequestURI()
				w.Header().Set("Content-Type", "application/json")
				response := EventResponse{Data: []Event{}, Page: 1, Limit: 10, Total: 0, LastPage: 0}
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client, _ := NewClient(Config{BaseURL: server.URL})
			ctx := context.Background()

			_, _ = client.GetEvents(ctx, tt.params)

			if tt.checkQuery != nil {
				tt.checkQuery(t, requestQuery)
			}
		})
	}
}

// TestConcurrentRequests tests that the client handles concurrent requests safely
func TestConcurrentRequests(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		time.Sleep(10 * time.Millisecond) // Simulate some processing time
		response := EventResponse{
			Data:     []Event{{Title: "Event", Location: "Venue", City: "City", URL: "https://e.com"}},
			Page:     1,
			Limit:    10,
			Total:    1,
			LastPage: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(Config{BaseURL: server.URL})

	// Launch multiple concurrent requests
	numRequests := 10
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			ctx := context.Background()
			_, err := client.GetEvents(ctx, QueryParams{City: "TestCity"})
			results <- err
		}()
	}

	// Collect results
	for i := 0; i < numRequests; i++ {
		if err := <-results; err != nil {
			t.Errorf("Concurrent request %d failed: %v", i, err)
		}
	}

	if requestCount != numRequests {
		t.Errorf("Expected %d requests, got %d", numRequests, requestCount)
	}
}

// TestDateParsing tests that various date formats are handled correctly
func TestDateParsing(t *testing.T) {
	tests := []struct {
		name         string
		dateString   string
		expectedTime time.Time
		wantErr      bool
	}{
		{
			name:         "ISO 8601 with timezone",
			dateString:   "2024-12-31T20:00:00Z",
			expectedTime: time.Date(2024, 12, 31, 20, 0, 0, 0, time.UTC),
			wantErr:      false,
		},
		{
			name:         "ISO 8601 with offset",
			dateString:   "2024-12-31T20:00:00+01:00",
			expectedTime: time.Date(2024, 12, 31, 19, 0, 0, 0, time.UTC),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := `{
				"data": [{
					"title": "Event",
					"location": "Venue",
					"city": "City",
					"date": "` + tt.dateString + `",
					"url": "https://example.com",
					"type": "concert",
					"sourceUrl": "https://example.com"
				}],
				"page": 1,
				"limit": 10,
				"total": 1,
				"last_page": 1
			}`

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(response))
			}))
			defer server.Close()

			client, _ := NewClient(Config{BaseURL: server.URL})
			ctx := context.Background()

			resp, err := client.GetEvents(ctx, QueryParams{City: "Test"})

			if (err != nil) != tt.wantErr {
				t.Errorf("GetEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if len(resp.Data) == 0 {
					t.Fatal("Expected at least one event")
				}
				eventTime := resp.Data[0].Date
				if !eventTime.Equal(tt.expectedTime) {
					t.Errorf("Expected time %v, got %v", tt.expectedTime, eventTime)
				}
			}
		})
	}
}

// TestPaginationScenarios tests various pagination scenarios
func TestPaginationScenarios(t *testing.T) {
	tests := []struct {
		name         string
		params       QueryParams
		responsePage int
		responseData int
		wantPage     int
		wantEvents   int
	}{
		{
			name:         "first page",
			params:       QueryParams{City: "Test", Page: 1, Limit: 10},
			responsePage: 1,
			responseData: 10,
			wantPage:     1,
			wantEvents:   10,
		},
		{
			name:         "middle page",
			params:       QueryParams{City: "Test", Page: 5, Limit: 20},
			responsePage: 5,
			responseData: 20,
			wantPage:     5,
			wantEvents:   20,
		},
		{
			name:         "last page partial",
			params:       QueryParams{City: "Test", Page: 10, Limit: 50},
			responsePage: 10,
			responseData: 23, // Partial last page
			wantPage:     10,
			wantEvents:   23,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				events := make([]Event, tt.responseData)
				for i := 0; i < tt.responseData; i++ {
					events[i] = Event{
						Title:     "Event",
						Location:  "Venue",
						City:      "City",
						URL:       "https://example.com",
						Type:      "concert",
						SourceURL: "https://example.com",
					}
				}

				response := EventResponse{
					Data:     events,
					Page:     tt.responsePage,
					Limit:    tt.params.Limit,
					Total:    100,
					LastPage: 10,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client, _ := NewClient(Config{BaseURL: server.URL})
			ctx := context.Background()

			resp, err := client.GetEvents(ctx, tt.params)

			if err != nil {
				t.Fatalf("GetEvents() error = %v", err)
			}

			if resp.Page != tt.wantPage {
				t.Errorf("Expected page %d, got %d", tt.wantPage, resp.Page)
			}

			if len(resp.Data) != tt.wantEvents {
				t.Errorf("Expected %d events, got %d", tt.wantEvents, len(resp.Data))
			}
		})
	}
}

// TestHTTPClientCustomization tests that custom HTTP clients work correctly
func TestHTTPClientCustomization(t *testing.T) {
	// Create a custom HTTP client with short timeout
	customClient := &http.Client{
		Timeout: 50 * time.Millisecond,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := NewClient(Config{
		BaseURL:    server.URL,
		HTTPClient: customClient,
	})

	ctx := context.Background()
	_, err := client.GetEvents(ctx, QueryParams{City: "Test"})

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

// TestResponseHeaders tests that response headers are handled correctly
func TestResponseHeaders(t *testing.T) {
	var contentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-RateLimit-Remaining", "100")
		response := EventResponse{Data: []Event{}, Page: 1, Limit: 10, Total: 0, LastPage: 0}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Capture response for inspection
	client, _ := NewClient(Config{
		BaseURL: server.URL,
		HTTPClient: &http.Client{
			Transport: &headerCapturingTransport{
				onResponse: func(resp *http.Response) {
					contentType = resp.Header.Get("Content-Type")
				},
			},
		},
	})

	ctx := context.Background()
	_, err := client.GetEvents(ctx, QueryParams{City: "Test"})

	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	if !contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type to contain 'application/json', got '%s'", contentType)
	}
}

// headerCapturingTransport is a custom transport for testing
type headerCapturingTransport struct {
	onResponse func(*http.Response)
}

func (t *headerCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil && t.onResponse != nil {
		t.onResponse(resp)
	}
	return resp, err
}
