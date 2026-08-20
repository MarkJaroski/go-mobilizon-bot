// mobilizon/client_test.go
package mobilizon

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

func strPtr(s string) *string { return &s }

// MockGraphQLClient for testing
type MockGraphQLClient struct {
	MakeRequestFunc func(
		ctx context.Context,
		req *graphql.Request,
		resp *graphql.Response,
	) error
}

func (m *MockGraphQLClient) MakeRequest(
	ctx context.Context,
	req *graphql.Request,
	resp *graphql.Response,
) error {
	if m.MakeRequestFunc != nil {
		return m.MakeRequestFunc(ctx, req, resp)
	}
	return nil
}

// clientWithMock creates a Client with the gqlClient field replaced by a mock.
func clientWithMock(mock *MockGraphQLClient) *Client {
	return &Client{
		baseURL:      "https://example.com",
		clientID:     "test-client-id",
		gqlClient:    mock,
		oauth2Config: &oauth2.Config{},
	}
}

// --- NewClient ---

func TestNewClient_EmptyClientID(t *testing.T) {
	_, err := NewClient("https://example.com", "")
	if err == nil {
		t.Fatal("expected error for empty clientID, got nil")
	}
}

func TestNewClient_Valid(t *testing.T) {
	c, err := NewClient("https://mob.example.com", "my-client-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.baseURL != "https://mob.example.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "https://mob.example.com")
	}
	if c.clientID != "my-client-id" {
		t.Errorf("clientID = %q, want %q", c.clientID, "my-client-id")
	}
}

// --- SaveToken / LoadToken ---

func TestSaveLoadToken_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	c := clientWithMock(nil)
	c.token = &oauth2.Token{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-xyz",
		TokenType:    "Bearer",
		Expiry:       time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := c.SaveToken(path); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	c2 := clientWithMock(nil)
	if err := c2.LoadToken(path); err != nil {
		t.Fatalf("LoadToken: %v", err)
	}

	if c2.token.AccessToken != "access-abc" {
		t.Errorf("AccessToken = %q, want %q", c2.token.AccessToken, "access-abc")
	}
	if c2.token.RefreshToken != "refresh-xyz" {
		t.Errorf("RefreshToken = %q, want %q", c2.token.RefreshToken, "refresh-xyz")
	}
}

func TestSaveToken_NilToken(t *testing.T) {
	c := clientWithMock(nil)
	err := c.SaveToken("/tmp/should-not-be-created.json")
	if err == nil {
		t.Fatal("expected error for nil token, got nil")
	}
}

func TestLoadToken_FileNotFound(t *testing.T) {
	c := clientWithMock(nil)
	err := c.LoadToken("/nonexistent/path/token.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadToken_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json {{{"), 0600)

	c := clientWithMock(nil)
	if err := c.LoadToken(path); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// --- RefreshToken ---

func TestRefreshToken_Success(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*RefreshAuthTokensResponse)
			data.RefreshToken = &RefreshAuthTokensRefreshTokenRefreshedToken{
				AccessToken:  "new-access",
				RefreshToken: "new-refresh",
			}
			return nil
		},
	}
	c := clientWithMock(mock)
	c.token = &oauth2.Token{RefreshToken: "old-refresh"}

	if err := c.RefreshToken(context.Background(), "/tmp/should-not-be-created.json"); err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if c.token.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want %q", c.token.AccessToken, "new-access")
	}
	if c.token.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q, want %q", c.token.RefreshToken, "new-refresh")
	}
}

func TestRefreshToken_Error(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			return errors.New("token expired")
		},
	}
	c := clientWithMock(mock)
	c.token = &oauth2.Token{RefreshToken: "old-refresh"}

	if err := c.RefreshToken(context.Background(), "/tmp/should-not-be-created.json"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- SearchForEvents ---

func TestSearchForEvents_Empty(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*SearchEventsResponse)
			data.SearchEvents = &SearchEventsSearchEvents{Total: 0, Elements: nil}
			return nil
		},
	}
	c := clientWithMock(mock)

	events, err := c.SearchForEvents(context.Background(), "nothing", time.Now())
	if err != nil {
		t.Fatalf("SearchForEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestSearchForEvents_MapsFields(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	beginsOn := time.Date(2025, 6, 15, 20, 0, 0, 0, time.UTC)

	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*SearchEventsResponse)
			beginsOnPlus1 := beginsOn.Add(time.Hour)
			data.SearchEvents = &SearchEventsSearchEvents{
				Total: 2,
				Elements: []*SearchEventsSearchEventsElementsEvent{
					{Id: strPtr("1"), Uuid: &id1, Title: strPtr("First Event"), BeginsOn: &beginsOn},
					{Id: strPtr("2"), Uuid: &id2, Title: strPtr("Second Event"), BeginsOn: &beginsOnPlus1},
				},
			}
			return nil
		},
	}
	c := clientWithMock(mock)

	events, err := c.SearchForEvents(context.Background(), "festival", beginsOn)
	if err != nil {
		t.Fatalf("SearchForEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != "1" || events[0].UUID != id1 || events[0].Title != "First Event" {
		t.Errorf("event[0] mismatch: %+v", events[0])
	}
	if events[1].ID != "2" || events[1].UUID != id2 || events[1].Title != "Second Event" {
		t.Errorf("event[1] mismatch: %+v", events[1])
	}
}

func TestSearchForEvents_Error(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			return errors.New("graphql error")
		},
	}
	c := clientWithMock(mock)

	_, err := c.SearchForEvents(context.Background(), "anything", time.Now())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- EventExists ---

func TestEventExists_NotFound(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*SearchEventsResponse)
			data.SearchEvents = &SearchEventsSearchEvents{Elements: nil}
			return nil
		},
	}
	c := clientWithMock(mock)

	exists, uid, err := c.EventExists(context.Background(), "My Concert", "Some Venue", time.Now())
	if err != nil {
		t.Fatalf("EventExists: %v", err)
	}
	if exists {
		t.Error("expected exists=false, got true")
	}
	if uid != nil {
		t.Errorf("expected nil UUID, got %v", uid)
	}
}

func TestEventExists_Found(t *testing.T) {
	expectedUUID := uuid.New()
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*SearchEventsResponse)
			data.SearchEvents = &SearchEventsSearchEvents{
				Elements: []*SearchEventsSearchEventsElementsEvent{
					{Id: strPtr("42"), Uuid: &expectedUUID, Title: strPtr("My Concert")},
				},
			}
			return nil
		},
	}
	c := clientWithMock(mock)

	exists, uid, err := c.EventExists(context.Background(), "My Concert", "Some Venue", time.Now())
	if err != nil {
		t.Fatalf("EventExists: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true, got false")
	}
	if uid == nil || *uid != expectedUUID {
		t.Errorf("UUID = %v, want %v", uid, expectedUUID)
	}
}

func TestEventExists_MultipleResults_ReturnsFirst(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*SearchEventsResponse)
			data.SearchEvents = &SearchEventsSearchEvents{
				Elements: []*SearchEventsSearchEventsElementsEvent{
					{Id: strPtr("1"), Uuid: &first},
					{Id: strPtr("2"), Uuid: &second},
				},
			}
			return nil
		},
	}
	c := clientWithMock(mock)

	exists, uid, err := c.EventExists(context.Background(), "Concert", "Venue", time.Now())
	if err != nil {
		t.Fatalf("EventExists: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}
	if uid == nil || *uid != first {
		t.Errorf("expected first UUID %v, got %v", first, uid)
	}
}

func TestEventExists_Error(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			return errors.New("search failed")
		},
	}
	c := clientWithMock(mock)

	exists, uid, err := c.EventExists(context.Background(), "Concert", "Venue", time.Now())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if exists || uid != nil {
		t.Error("expected false/nil on error")
	}
}

// --- FetchAddr ---

func TestFetchAddr_MapsFields(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*SearchAddressResponse)
			data.SearchAddress = []*SearchAddressSearchAddress{
				{
					AdressFragment: AdressFragment{
						Id:          strPtr("addr-1"),
						Street:      strPtr("1 Main St"),
						Locality:    strPtr("Lausanne"),
						PostalCode:  strPtr("1000"),
						Region:      strPtr("Vaud"),
						Country:     strPtr("Switzerland"),
						Description: strPtr("Test Venue"),
						Timezone:    strPtr("Europe/Zurich"),
						Geom:        strPtr("6.6323 46.5197"),
						OriginId:    strPtr("origin-1"),
					},
				},
			}
			return nil
		},
	}
	c := clientWithMock(mock)

	addrs, err := c.FetchAddr(context.Background(), "Test Venue Lausanne")
	if err != nil {
		t.Fatalf("FetchAddr: %v", err)
	}
	if len(addrs) != 1 {
		t.Fatalf("expected 1 address, got %d", len(addrs))
	}
	a := addrs[0]
	if a.Street == nil || *a.Street != "1 Main St" {
		t.Errorf("Street = %v, want %q", a.Street, "1 Main St")
	}
	if a.Locality == nil || *a.Locality != "Lausanne" {
		t.Errorf("Locality = %v, want %q", a.Locality, "Lausanne")
	}
	if a.PostalCode == nil || *a.PostalCode != "1000" {
		t.Errorf("PostalCode = %v, want %q", a.PostalCode, "1000")
	}
	if a.Country == nil || *a.Country != "Switzerland" {
		t.Errorf("Country = %v, want %q", a.Country, "Switzerland")
	}
	if a.Timezone == nil || *a.Timezone != "Europe/Zurich" {
		t.Errorf("Timezone = %v, want %q", a.Timezone, "Europe/Zurich")
	}
}

func TestFetchAddr_Empty(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*SearchAddressResponse)
			data.SearchAddress = nil
			return nil
		},
	}
	c := clientWithMock(mock)

	addrs, err := c.FetchAddr(context.Background(), "nowhere")
	if err != nil {
		t.Fatalf("FetchAddr: %v", err)
	}
	if len(addrs) != 0 {
		t.Errorf("expected 0 addresses, got %d", len(addrs))
	}
}

func TestFetchAddr_Error(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			return errors.New("address lookup failed")
		},
	}
	c := clientWithMock(mock)

	_, err := c.FetchAddr(context.Background(), "somewhere")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- CreateEvent ---

func TestCreateEvent_Success(t *testing.T) {
	expectedUUID := uuid.New()
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			data := resp.Data.(*CreateEventResponse)
			data.CreateEvent = &CreateEventCreateEvent{
				Id:   strPtr("99"),
				Uuid: &expectedUUID,
			}
			return nil
		},
	}
	c := clientWithMock(mock)

	params := EventParams{
		Title:            "Test Concert",
		Description:      "A great show",
		BeginsOn:         time.Date(2025, 12, 31, 20, 0, 0, 0, time.UTC),
		EndsOn:           time.Date(2025, 12, 31, 22, 0, 0, 0, time.UTC),
		OrganizerActorId: 1,
		AttributedToId:   2,
		Visibility:       EventVisibilityPublic,
		JoinOptions:      EventJoinOptionsExternal,
	}

	uid, err := c.CreateEvent(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if uid == nil || *uid != expectedUUID {
		t.Errorf("UUID = %v, want %v", uid, expectedUUID)
	}
}

func TestCreateEvent_Error(t *testing.T) {
	mock := &MockGraphQLClient{
		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
			return errors.New("mutation failed")
		},
	}
	c := clientWithMock(mock)

	_, err := c.CreateEvent(context.Background(), EventParams{
		Title:    "Test",
		BeginsOn: time.Now(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- RetryPolicy ---

func TestRetryPolicy_401_ShouldRetry(t *testing.T) {
	resp := &http.Response{Status: "401"}
	retry, err := RetryPolicy(context.Background(), resp, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !retry {
		t.Error("expected retry=true for 401")
	}
}

func TestRetryPolicy_200_NoRetry(t *testing.T) {
	resp := &http.Response{Status: "200"}
	retry, _ := RetryPolicy(context.Background(), resp, nil)
	if retry {
		t.Error("expected retry=false for 200")
	}
}

func TestRetryPolicy_404_NoRetry(t *testing.T) {
	resp := &http.Response{Status: "404"}
	retry, _ := RetryPolicy(context.Background(), resp, nil)
	if retry {
		t.Error("expected retry=false for 404")
	}
}

func TestRetryPolicy_503_ShouldRetry(t *testing.T) {
	resp := &http.Response{Status: "503"}
	retry, _ := RetryPolicy(context.Background(), resp, nil)
	if !retry {
		t.Error("expected retry=true for 503")
	}
}

func TestRetryPolicy_405_ShouldRetry(t *testing.T) {
	resp := &http.Response{Status: "405"}
	retry, _ := RetryPolicy(context.Background(), resp, nil)
	if !retry {
		t.Error("expected retry=true for 405")
	}
}

// --- ErrorBackoff ---

func TestErrorBackoff_ReturnsConstant(t *testing.T) {
	d := ErrorBackoff(0, 0, 0, nil)
	if d != SERVER_CRASH_WAIT_TIME {
		t.Errorf("ErrorBackoff = %v, want %v", d, SERVER_CRASH_WAIT_TIME)
	}
}
