// concertcloud/client_test.go
package concertcloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
)

// TestNewClient tests client creation
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantURL string
		wantErr bool
	}{
		{
			name: "default configuration",
			config: Config{
				BaseURL: "",
			},
			wantURL: DefaultBaseURL,
			wantErr: false,
		},
		{
			name: "custom base URL",
			config: Config{
				BaseURL: "https://custom.api.example.com",
			},
			wantURL: "https://custom.api.example.com",
			wantErr: false,
		},
		{
			name: "with logger",
			config: Config{
				BaseURL: "https://api.example.com",
				Logger:  hclog.NewNullLogger(),
			},
			wantURL: "https://api.example.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && client.baseURL != tt.wantURL {
				t.Errorf("NewClient() baseURL = %v, want %v", client.baseURL, tt.wantURL)
			}
			if err == nil && client.httpClient == nil {
				t.Error("NewClient() httpClient should not be nil")
			}
		})
	}
}

func TestClient_GetEvents(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameters
		city := r.URL.Query().Get("city")
		if city != "Lausanne" {
			t.Errorf("Expected city=Lausanne, got %s", city)
		}

		// Return mock response
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
            "data": [
                {
                    "title": "Test Event",
                    "location": "Test Venue",
                    "city": "Lausanne",
                    "date": "2024-12-31T20:00:00Z",
                    "url": "https://example.com/event"
                }
            ],
            "page": 1,
            "limit": 10,
            "total": 1,
            "last_page": 1
        }`))
	}))
	defer server.Close()

	// Create client with test server URL
	client, err := NewClient(Config{
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Test GetEvents
	resp, err := client.GetEventsByCity(context.Background(), "Lausanne", 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.Data) != 1 {
		t.Errorf("Expected 1 event, got %d", len(resp.Data))
	}

	if resp.Data[0].Title != "Test Event" {
		t.Errorf("Expected title 'Test Event', got '%s'", resp.Data[0].Title)
	}
}

// TestClient_GetEventsByCity tests the convenience method for city queries
func TestClient_GetEventsByCity(t *testing.T) {
	tests := []struct {
		name         string
		city         string
		limit        int
		wantEvents   int
		wantErr      bool
		checkRequest func(t *testing.T, r *http.Request)
	}{
		{
			name:       "valid city query",
			city:       "Lausanne",
			limit:      50,
			wantEvents: 2,
			wantErr:    false,
			checkRequest: func(t *testing.T, r *http.Request) {
				city := r.URL.Query().Get("city")
				if city != "Lausanne" {
					t.Errorf("Expected city=Lausanne, got city=%s", city)
				}
				limit := r.URL.Query().Get("limit")
				if limit != "50" {
					t.Errorf("Expected limit=50, got limit=%s", limit)
				}
			},
		},
		{
			name:       "city with spaces",
			city:       "New York",
			limit:      10,
			wantEvents: 1,
			wantErr:    false,
			checkRequest: func(t *testing.T, r *http.Request) {
				city := r.URL.Query().Get("city")
				if city != "New York" {
					t.Errorf("Expected city='New York', got city=%s", city)
				}
			},
		},
		{
			name:       "city with special characters",
			city:       "São Paulo",
			limit:      100,
			wantEvents: 0,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.checkRequest != nil {
					tt.checkRequest(t, r)
				}

				response := EventResponse{
					Data:     make([]Event, tt.wantEvents),
					Page:     1,
					Limit:    tt.limit,
					Total:    tt.wantEvents,
					LastPage: 1,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client, _ := NewClient(Config{
				BaseURL: server.URL,
			})

			ctx := context.Background()
			resp, err := client.GetEventsByCity(ctx, tt.city, tt.limit)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetEventsByCity() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && len(resp.Data) != tt.wantEvents {
				t.Errorf("GetEventsByCity() got %d events, want %d", len(resp.Data), tt.wantEvents)
			}
		})
	}
}

// TestClient_GetEventsByCountry tests the convenience method for country queries
func TestClient_GetEventsByCountry(t *testing.T) {
	tests := []struct {
		name         string
		country      string
		limit        int
		wantEvents   int
		wantErr      bool
		checkRequest func(t *testing.T, r *http.Request)
	}{
		{
			name:       "valid country query",
			country:    "Switzerland",
			limit:      100,
			wantEvents: 5,
			wantErr:    false,
			checkRequest: func(t *testing.T, r *http.Request) {
				country := r.URL.Query().Get("country")
				if country != "Switzerland" {
					t.Errorf("Expected country=Switzerland, got country=%s", country)
				}
				limit := r.URL.Query().Get("limit")
				if limit != "100" {
					t.Errorf("Expected limit=100, got limit=%s", limit)
				}
			},
		},
		{
			name:       "country with spaces",
			country:    "United States",
			limit:      200,
			wantEvents: 10,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.checkRequest != nil {
					tt.checkRequest(t, r)
				}

				response := EventResponse{
					Data:     make([]Event, tt.wantEvents),
					Page:     1,
					Limit:    tt.limit,
					Total:    tt.wantEvents,
					LastPage: 1,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client, _ := NewClient(Config{
				BaseURL: server.URL,
			})

			ctx := context.Background()
			resp, err := client.GetEventsByCountry(ctx, tt.country, tt.limit)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetEventsByCountry() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && len(resp.Data) != tt.wantEvents {
				t.Errorf("GetEventsByCountry() got %d events, want %d", len(resp.Data), tt.wantEvents)
			}
		})
	}
}

// TestClient_GetEventsWithContext tests context cancellation
func TestClient_GetEventsWithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": [], "page": 1, "limit": 10, "total": 0, "last_page": 0}`))
	}))
	defer server.Close()

	client, _ := NewClient(Config{
		BaseURL: server.URL,
	})

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.GetEvents(ctx, QueryParams{City: "TestCity"})

	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

// TestClient_GetEventsWithRealData tests parsing of realistic event data
func TestClient_GetEventsWithRealData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"data": [
				{
					"title": "DJ Snake Live",
					"location": "Paléo Festival",
					"city": "Nyon",
					"country": "Switzerland",
					"date": "2024-07-20T22:00:00Z",
					"offset": 120,
					"url": "https://www.paleo.ch/en/program/dj-snake",
					"imageUrl": "https://example.com/djsnake.jpg",
					"comment": "One of the world's biggest EDM artists",
					"type": "concert",
					"sourceUrl": "https://www.paleo.ch",
					"genres": ["EDM", "Hip Hop", "Trap"],
					"genresText": "Electronic Dance Music with Hip Hop influences",
					"address": {
						"locality": "Nyon",
						"postCode": "1260",
						"street": "Route de l'Asse",
						"country": "Switzerland",
						"geolocation": {
							"type": "Point",
							"coordinates": [6.2391, 46.3833]
						}
					}
				}
			],
			"page": 1,
			"limit": 10,
			"total": 1,
			"last_page": 1
		}`

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	defer server.Close()

	client, _ := NewClient(Config{
		BaseURL: server.URL,
	})

	ctx := context.Background()
	resp, err := client.GetEvents(ctx, QueryParams{City: "Nyon"})

	if err != nil {
		t.Fatalf("GetEvents() error = %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(resp.Data))
	}

	event := resp.Data[0]

	// Validate event fields
	if event.Title != "DJ Snake Live" {
		t.Errorf("Expected title 'DJ Snake Live', got '%s'", event.Title)
	}

	if event.Location != "Paléo Festival" {
		t.Errorf("Expected location 'Paléo Festival', got '%s'", event.Location)
	}

	if event.City != "Nyon" {
		t.Errorf("Expected city 'Nyon', got '%s'", event.City)
	}

	if event.Country != "Switzerland" {
		t.Errorf("Expected country 'Switzerland', got '%s'", event.Country)
	}

	if event.Type != "concert" {
		t.Errorf("Expected type 'concert', got '%s'", event.Type)
	}

	if len(event.Genres) != 3 {
		t.Errorf("Expected 3 genres, got %d", len(event.Genres))
	}

	expectedGenres := []string{"EDM", "Hip Hop", "Trap"}
	for i, genre := range expectedGenres {
		if event.Genres[i] != genre {
			t.Errorf("Expected genre[%d] = '%s', got '%s'", i, genre, event.Genres[i])
		}
	}

	if event.Address.Locality != "Nyon" {
		t.Errorf("Expected address locality 'Nyon', got '%s'", event.Address.Locality)
	}

	if event.Address.PostCode != "1260" {
		t.Errorf("Expected postcode '1260', got '%s'", event.Address.PostCode)
	}
}

// TestClient_GetEventsErrorScenarios tests various error scenarios
func TestClient_GetEventsErrorScenarios(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *httptest.Server
		expectedError string
	}{
		{
			name: "network timeout",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(200 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
				}))
			},
			expectedError: "context deadline exceeded",
		},
		{
			name: "malformed JSON",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"data": [invalid json]}`))
				}))
			},
			expectedError: "failed to decode response",
		},
		{
			name: "API rate limit",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusTooManyRequests)
					w.Write([]byte(`{"error": "Rate limit exceeded"}`))
				}))
			},
			expectedError: "API returned status 429",
		},
		{
			name: "API maintenance",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
					w.Write([]byte(`{"error": "Service temporarily unavailable"}`))
				}))
			},
			expectedError: "API returned status 503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			defer server.Close()

			client, _ := NewClient(Config{
				BaseURL: server.URL,
			})

			ctx := context.Background()
			if tt.name == "network timeout" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
			}

			_, err := client.GetEvents(ctx, QueryParams{City: "TestCity"})

			if err == nil {
				t.Error("Expected error, got nil")
				return
			}

			if !contains(err.Error(), tt.expectedError) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedError, err.Error())
			}
		})
	}
}

// BenchmarkClient_GetEvents benchmarks the GetEvents method
func BenchmarkClient_GetEvents(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := EventResponse{
			Data: []Event{
				{
					Title:    "Test Event",
					Location: "Test Venue",
					City:     "Test City",
					URL:      "https://example.com/event",
				},
			},
			Page:     1,
			Limit:    10,
			Total:    1,
			LastPage: 1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client, _ := NewClient(Config{
		BaseURL: server.URL,
	})

	ctx := context.Background()
	params := QueryParams{City: "TestCity", Limit: 10}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.GetEvents(ctx, params)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
