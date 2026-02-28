//go:build integration

package concertcloud_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/markjaroski/go-mobilizon-bot/concertcloud"
)

// TestIntegration_RealAPI tests against the actual ConcertCloud API
// This test is skipped by default and only runs when the integration build tag is set
func TestIntegration_RealAPI(t *testing.T) {
	// Skip if running in CI without real API access
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	client, err := concertcloud.NewClient(concertcloud.Config{
		BaseURL: "", // Use default production URL
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("fetch events by city", func(t *testing.T) {
		resp, err := client.GetEventsByCity(ctx, "Lausanne", 10)
		if err != nil {
			t.Fatalf("GetEventsByCity failed: %v", err)
		}

		t.Logf("Found %d events (total: %d)", len(resp.Data), resp.Total)

		// Validate that we got a reasonable response
		if resp.Page < 1 {
			t.Errorf("Invalid page number: %d", resp.Page)
		}

		if resp.Limit != 10 {
			t.Errorf("Expected limit 10, got %d", resp.Limit)
		}

		// Check event structure
		for i, event := range resp.Data {
			if event.Title == "" {
				t.Errorf("Event %d has empty title", i)
			}
			if event.URL == "" {
				t.Errorf("Event %d has empty URL", i)
			}
			if event.Date.IsZero() {
				t.Errorf("Event %d has zero date", i)
			}
		}
	})

	t.Run("fetch events by country", func(t *testing.T) {
		resp, err := client.GetEventsByCountry(ctx, "Switzerland", 50)
		if err != nil {
			t.Fatalf("GetEventsByCountry failed: %v", err)
		}

		t.Logf("Found %d events in Switzerland (total: %d)", len(resp.Data), resp.Total)

		// Should get some events
		if resp.Total == 0 {
			t.Log("Warning: No events found in Switzerland (unexpected)")
		}

		// Validate country field if present
		for i, event := range resp.Data {
			if event.Country != "" && event.Country != "Switzerland" {
				t.Errorf("Event %d has unexpected country: %s", i, event.Country)
			}
		}
	})

	t.Run("pagination", func(t *testing.T) {
		// Get first page
		page1, err := client.GetEvents(ctx, concertcloud.QueryParams{
			City:  "Zurich",
			Limit: 5,
			Page:  1,
		})
		if err != nil {
			t.Fatalf("Failed to get page 1: %v", err)
		}

		t.Logf("Page 1: %d events, total: %d, last page: %d",
			len(page1.Data), page1.Total, page1.LastPage)

		// If there are more pages, get page 2
		if page1.LastPage > 1 {
			page2, err := client.GetEvents(ctx, concertcloud.QueryParams{
				City:  "Zurich",
				Limit: 5,
				Page:  2,
			})
			if err != nil {
				t.Fatalf("Failed to get page 2: %v", err)
			}

			t.Logf("Page 2: %d events", len(page2.Data))

			// Verify we got different events
			if len(page1.Data) > 0 && len(page2.Data) > 0 {
				if page1.Data[0].URL == page2.Data[0].URL {
					t.Error("Page 1 and Page 2 returned the same events")
				}
			}
		}
	})

	t.Run("empty city returns no events or error", func(t *testing.T) {
		resp, err := client.GetEventsByCity(ctx, "NonExistentCityXYZ123", 10)
		if err != nil {
			t.Logf("Empty city returned error (acceptable): %v", err)
			return
		}

		if len(resp.Data) > 0 {
			t.Logf("Unexpectedly found %d events for non-existent city", len(resp.Data))
		}
	})

	t.Run("date filter", func(t *testing.T) {
		// Test future date filter
		futureDate := time.Now().AddDate(0, 6, 0).Format("2006-01-02T15:04:05Z07:00")

		resp, err := client.GetEvents(ctx, concertcloud.QueryParams{
			City:  "Geneva",
			Date:  futureDate,
			Limit: 20,
		})
		if err != nil {
			t.Fatalf("Date filter failed: %v", err)
		}

		t.Logf("Found %d events after %s", len(resp.Data), futureDate)

		// All returned events should be on or after the filter date
		filterTime, _ := time.Parse("2006-01-02", futureDate)
		for i, event := range resp.Data {
			if event.Date.Before(filterTime) {
				t.Errorf("Event %d (%s) is before filter date %s",
					i, event.Date.Format("2006-01-02"), futureDate)
			}
		}
	})
}

// TestIntegration_APIAvailability just checks if the API is reachable
func TestIntegration_APIAvailability(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	client, err := concertcloud.NewClient(concertcloud.Config{})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Simple query to check if API is up
	_, err = client.GetEvents(ctx, concertcloud.QueryParams{
		Limit: 1,
	})

	if err != nil {
		t.Fatalf("API appears to be unavailable: %v", err)
	}

	t.Log("✓ ConcertCloud API is available")
}

// TestIntegration_RateLimiting tests how the client handles rate limiting
// This test should be run carefully to avoid actually hitting rate limits
func TestIntegration_RateLimiting(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	// Only run this test if explicitly requested
	if os.Getenv("TEST_RATE_LIMITING") != "true" {
		t.Skip("Skipping rate limit test (set TEST_RATE_LIMITING=true to run)")
	}

	client, err := concertcloud.NewClient(concertcloud.Config{})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	// Make many rapid requests
	const numRequests = 20
	errors := 0

	for i := range numRequests {
		_, err := client.GetEvents(ctx, concertcloud.QueryParams{
			City:  "Lausanne",
			Limit: 1,
		})

		if err != nil {
			t.Logf("Request %d failed: %v", i+1, err)
			errors++
		}

		// Small delay to avoid hammering the API too hard
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Made %d requests, %d failed", numRequests, errors)

	// We don't fail the test even if we hit rate limits
	// Just log the behavior
	if errors > 0 {
		t.Logf("Client encountered errors (possibly rate limiting)")
	}
}

// TestIntegration_EventDataQuality checks the quality of real API data
func TestIntegration_EventDataQuality(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration test (SKIP_INTEGRATION=true)")
	}

	client, err := concertcloud.NewClient(concertcloud.Config{})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.GetEventsByCountry(ctx, "Switzerland", 100)
	if err != nil {
		t.Fatalf("Failed to fetch events: %v", err)
	}

	if len(resp.Data) == 0 {
		t.Skip("No events available for quality check")
	}

	var stats struct {
		withImages      int
		withGenres      int
		withComments    int
		withAddress     int
		withCoordinates int
		pastEvents      int
		futureEvents    int
	}

	now := time.Now()

	for _, event := range resp.Data {
		if event.ImageURL != "" {
			stats.withImages++
		}
		if len(event.Genres) > 0 {
			stats.withGenres++
		}
		if event.Comment != "" {
			stats.withComments++
		}
		if event.Address.Locality != "" {
			stats.withAddress++
		}
		if len(event.Address.Geolocacation.Coordinates) == 2 {
			stats.withCoordinates++
		}
		if event.Date.Before(now) {
			stats.pastEvents++
		} else {
			stats.futureEvents++
		}
	}

	total := len(resp.Data)

	t.Logf("Data Quality Stats (n=%d):", total)
	t.Logf("  Events with images: %d (%.1f%%)", stats.withImages, 100*float64(stats.withImages)/float64(total))
	t.Logf("  Events with genres: %d (%.1f%%)", stats.withGenres, 100*float64(stats.withGenres)/float64(total))
	t.Logf("  Events with comments: %d (%.1f%%)", stats.withComments, 100*float64(stats.withComments)/float64(total))
	t.Logf("  Events with address: %d (%.1f%%)", stats.withAddress, 100*float64(stats.withAddress)/float64(total))
	t.Logf("  Events with coordinates: %d (%.1f%%)", stats.withCoordinates, 100*float64(stats.withCoordinates)/float64(total))
	t.Logf("  Past events: %d", stats.pastEvents)
	t.Logf("  Future events: %d", stats.futureEvents)

	// Just informational, don't fail
}
