# go-mobilizon-bot Refactoring Plan

## Overview
This document outlines a plan to refactor the `go-mobilizon-bot` project by extracting the Mobilizon interaction code from `bot.go` into a separate, testable library.

## Current State Analysis

### bot.go Structure (1410 lines)
The file currently contains:
1. **Type definitions** (lines 1-270): GraphQL types, event types, configuration
2. **Global variables** (lines 271-330): Logger, clients, auth state
3. **Main function** (lines 331-498): CLI parsing and orchestration
4. **Mobilizon API functions** (lines 499-1410): Core library candidates
5. **Helper utilities**: Image processing, HTTP handling, data fetching

### Key Mobilizon Functions to Extract

#### Authentication & Authorization
- `registerApp()` (lines ~500-580): Device registration flow
- `authorizeApp()` (lines ~580-660): OAuth device code flow
- `refreshAuthorization()` (lines 1295-1342): Token refresh

#### Event Management
- `createMobilizonEvent()` (lines ~660-840): Create events via GraphQL
- `eventExists()` (lines 1228-1291): Search and deduplication
- `updateMobilizonEvent()` (lines ~840-920): Update existing events

#### Media Handling
- `uploadMobilizonMedia()` (lines ~1060-1140): Upload images
- `prepareMobilizonMediaUpload()` (lines 1140-1181): Prepare multipart uploads
- `downloadFile()` (lines 1183-1226): Download and process images
- `thumbnail()` (lines 1344-1383): Image resizing

#### Address/Location
- `getMobilizonAddresses()` (lines ~920-1000): Location search
- `createMobilizonAddress()` (lines ~1000-1060): Address creation

## Refactoring Strategy

### Phase 1: Create Library Package Structure

```
go-mobilizon-bot/
├── bot.go                    # Slim CLI application
├── mobilizon/               # Mobilizon API library package
│   ├── client.go            # Main client struct
│   ├── auth.go              # Authentication functions
│   ├── events.go            # Event operations
│   ├── media.go             # Media upload/download
│   ├── addresses.go         # Address/location functions
│   ├── types.go             # GraphQL type definitions
│   └── errors.go            # Custom error types
├── concertcloud/            # ConcertCloud/event-api library package
│   ├── client.go            # REST API client
│   ├── events.go            # Event fetching
│   ├── types.go             # Event types (from event-api)
│   └── errors.go            # Error types
├── mobilizon_test/          # Tests for Mobilizon library
│   ├── client_test.go
│   ├── auth_test.go
│   ├── events_test.go
│   ├── media_test.go
│   └── mocks.go             # Mock implementations
├── concertcloud_test/       # Tests for ConcertCloud library
│   ├── client_test.go
│   ├── events_test.go
│   └── mocks.go             # Mock HTTP responses
└── internal/                # Internal helpers (if needed)
    └── utils/
        ├── images.go        # Image processing
        └── http.go          # HTTP utilities
```

### Phase 2: Design the Client Interface

#### Mobilizon Client

```go
// mobilizon/client.go
package mobilizon

import (
    "context"
    "github.com/hasura/go-graphql-client"
    "github.com/hashicorp/go-hclog"
)

// Client represents a Mobilizon API client
type Client struct {
    baseURL    string
    gqlClient  *graphql.Client
    httpClient *http.Client
    auth       *AuthConfig
    logger     hclog.Logger
}

// Config holds client configuration
type Config struct {
    BaseURL        string
    Logger         hclog.Logger
    HTTPClient     *http.Client
    AuthConfigPath string
}

// NewClient creates a new Mobilizon client
func NewClient(config Config) (*Client, error) {
    // Implementation
}

// WithAuth sets authentication for the client
func (c *Client) WithAuth(auth *AuthConfig) *Client {
    c.auth = auth
    return c
}
```

#### ConcertCloud Client

```go
// concertcloud/client.go
package concertcloud

import (
    "context"
    "net/http"
    "github.com/hashicorp/go-hclog"
    "github.com/jakopako/event-api/models"
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
```

### Phase 3: Define Core Interfaces

#### Mobilizon Interfaces

```go
// mobilizon/types.go
package mobilizon

import "time"

// EventCreator defines the interface for creating events
type EventCreator interface {
    CreateEvent(ctx context.Context, params CreateEventParams) (*Event, error)
    UpdateEvent(ctx context.Context, uuid string, params UpdateEventParams) (*Event, error)
}

// EventSearcher defines the interface for searching events
type EventSearcher interface {
    SearchEvents(ctx context.Context, term string, beginsOn time.Time) ([]Event, error)
    EventExists(ctx context.Context, title string, url string, date time.Time) (bool, string, error)
}

// MediaUploader defines the interface for media operations
type MediaUploader interface {
    UploadMedia(ctx context.Context, filepath string) (*MediaUpload, error)
    DownloadFile(ctx context.Context, url string) (string, error)
}

// AddressManager defines the interface for address operations
type AddressManager interface {
    SearchAddress(ctx context.Context, query string) ([]Address, error)
    CreateAddress(ctx context.Context, input AddressInput) (*Address, error)
}

// Authenticator defines the interface for authentication
type Authenticator interface {
    Register(ctx context.Context) (*AppRegistration, error)
    Authorize(ctx context.Context, clientID string) (*AuthConfig, error)
    RefreshToken(ctx context.Context) error
}
```

#### ConcertCloud Interfaces

```go
// concertcloud/types.go
package concertcloud

import (
    "time"
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
```

### Phase 4: Extract Type Definitions

Move all GraphQL and domain types to `mobilizon/types.go`:

```go
// mobilizon/types.go
package mobilizon

import "time"

// UUID represents the GraphQL UUID type
type UUID string

// Event represents a Mobilizon event
type Event struct {
    ID          string
    UUID        string
    Title       string
    Description string
    BeginsOn    time.Time
    EndsOn      time.Time
    // ... other fields
}

// CreateEventParams holds parameters for creating an event
type CreateEventParams struct {
    Title         string
    Description   string
    BeginsOn      time.Time
    EndsOn        *time.Time
    Category      EventCategory
    Visibility    EventVisibility
    JoinOptions   EventJoinOptions
    AttributedTo  UUID
    OrganizedBy   UUID
    PhysicalAddr  *AddressInput
    PictureID     *UUID
    OnlineAddress string
    Draft         bool
}

// UpdateEventParams holds parameters for updating an event
type UpdateEventParams struct {
    // Similar to CreateEventParams but with optional fields
}

// AuthConfig holds authentication tokens
type AuthConfig struct {
    AccessToken  string `json:"accessToken"`
    RefreshToken string `json:"refreshToken"`
    ExpiresIn    int    `json:"expiresIn"`
}

// MediaUpload represents an uploaded media file
type MediaUpload struct {
    UUID UUID
    URL  string
}

// Address represents a physical location
type Address struct {
    ID          int
    Description string
    Locality    string
    PostalCode  string
    Street      string
    Country     string
    Region      string
    Latitude    float64
    Longitude   float64
}

// AddressInput represents address data for mutations
type AddressInput struct {
    Description string
    Locality    string
    PostalCode  string
    Street      string
    Country     string
    Region      string
    Geom        Point
}
```

### Phase 5: Implement Authentication Module

```go
// mobilizon/auth.go
package mobilizon

import (
    "context"
    "encoding/json"
    "os"
)

// Register registers the application with Mobilizon
func (c *Client) Register(ctx context.Context) (*AppRegistration, error) {
    // Extract logic from registerApp()
    // Return structured data instead of printing
}

// Authorize performs the OAuth device code flow
func (c *Client) Authorize(ctx context.Context, clientID string) (*AuthConfig, error) {
    // Extract logic from authorizeApp()
    // Make prompts injectable for testing
}

// RefreshToken refreshes the access token using the refresh token
func (c *Client) RefreshToken(ctx context.Context) error {
    // Extract logic from refreshAuthorization()
    // Don't read/write files directly - let caller handle persistence
}

// LoadAuthConfig loads auth config from file
func LoadAuthConfig(path string) (*AuthConfig, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    var auth AuthConfig
    if err := json.Unmarshal(data, &auth); err != nil {
        return nil, err
    }
    
    return &auth, nil
}

// SaveAuthConfig saves auth config to file
func SaveAuthConfig(path string, auth *AuthConfig) error {
    data, err := json.MarshalIndent(auth, "", "  ")
    if err != nil {
        return err
    }
    
    return os.WriteFile(path, data, 0600)
}
```

### Phase 6: Implement Event Operations

```go
// mobilizon/events.go
package mobilizon

import (
    "context"
    "github.com/hasura/go-graphql-client"
)

// CreateEvent creates a new event in Mobilizon
func (c *Client) CreateEvent(ctx context.Context, params CreateEventParams) (*Event, error) {
    // Extract logic from createMobilizonEvent()
    // Return structured response instead of logging
    
    var mutation struct {
        CreateEvent struct {
            ID    graphql.ID
            UUID  string
            Title string
            // ... other fields
        } `graphql:"createEvent(...)"`
    }
    
    variables := map[string]interface{}{
        // Build from params
    }
    
    if err := c.gqlClient.Mutate(ctx, &mutation, variables); err != nil {
        return nil, err
    }
    
    return &Event{
        ID:    string(mutation.CreateEvent.ID),
        UUID:  mutation.CreateEvent.UUID,
        Title: mutation.CreateEvent.Title,
    }, nil
}

// SearchEvents searches for events by term and date
func (c *Client) SearchEvents(ctx context.Context, term string, beginsOn time.Time) ([]Event, error) {
    // Extract from eventExists()
}

// EventExists checks if an event exists by matching title, date, and URL
func (c *Client) EventExists(ctx context.Context, title, url string, date time.Time) (bool, string, error) {
    // Extract logic from eventExists()
    // Return (exists bool, uuid string, error)
}

// UpdateEvent updates an existing event
func (c *Client) UpdateEvent(ctx context.Context, uuid string, params UpdateEventParams) (*Event, error) {
    // Extract from updateMobilizonEvent()
}
```

### Phase 7: Implement Media Operations

```go
// mobilizon/media.go
package mobilizon

import (
    "context"
    "io"
    "net/http"
)

// UploadMedia uploads a media file to Mobilizon
func (c *Client) UploadMedia(ctx context.Context, filepath string) (*MediaUpload, error) {
    // Extract from uploadMobilizonMedia() and prepareMobilizonMediaUpload()
}

// UploadMediaFromURL downloads and uploads a media file
func (c *Client) UploadMediaFromURL(ctx context.Context, url string) (*MediaUpload, error) {
    // Combine downloadFile() and uploadMobilizonMedia()
}

// DownloadFile downloads a file from URL
func (c *Client) DownloadFile(ctx context.Context, url string) (string, error) {
    // Extract from downloadFile()
}

// ProcessImage processes an image (resize, convert)
func ProcessImage(r io.Reader, w io.Writer, opts ImageProcessOptions) error {
    // Extract from thumbnail()
}

type ImageProcessOptions struct {
    Width     int
    MimeType  string
    MaxSize   int64
}
```

### Phase 8: Implement Address Operations

```go
// mobilizon/addresses.go
package mobilizon

import "context"

// SearchAddress searches for addresses via OpenStreetMap Nominatim
func (c *Client) SearchAddress(ctx context.Context, query string) ([]Address, error) {
    // Extract from getMobilizonAddresses()
}

// CreateAddress creates a new address in Mobilizon
func (c *Client) CreateAddress(ctx context.Context, input AddressInput) (*Address, error) {
    // Extract from createMobilizonAddress()
}
```

### Phase 9: Implement ConcertCloud Client

This is a new phase for extracting the REST API interactions with ConcertCloud.

```go
// concertcloud/client.go
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

// Client represents a ConcertCloud API client
type Client struct {
    baseURL    string
    httpClient *http.Client
    logger     hclog.Logger
}

// Config holds client configuration
type Config struct {
    BaseURL    string
    HTTPClient *http.Client
    Logger     hclog.Logger
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

// GetEvents fetches events from the ConcertCloud API
func (c *Client) GetEvents(ctx context.Context, params QueryParams) (*EventResponse, error) {
    // Build query parameters
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
    
    // Build URL
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
```

```go
// concertcloud/types.go
package concertcloud

import (
    "github.com/jakopako/event-api/models"
)

// Event is an alias for models.Event from event-api
// This allows us to use the standard event-api types
type Event = models.Event

// EventResponse represents the paginated API response for event queries
// This matches the Response type from the ConcertCloud API
type EventResponse struct {
    Data     []Event `json:"data"`
    Page     int     `json:"page"`
    Limit    int     `json:"limit"`
    Total    int     `json:"total"`
    LastPage int     `json:"last_page"`
}

// QueryParams holds parameters for event API queries
type QueryParams struct {
    City    string // Filter by city name
    Country string // Filter by country name
    Limit   int    // Maximum number of results
    Page    int    // Page number for pagination
    Radius  int    // Search radius in km (if supported)
    Date    string // Date filter (format varies by API)
}
```

```go
// concertcloud/errors.go
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
```

### Phase 10: Add Error Handling

```go
// mobilizon/errors.go
package mobilizon

import "errors"

var (
    ErrUnauthorized     = errors.New("unauthorized: invalid or expired token")
    ErrNotFound         = errors.New("resource not found")
    ErrInvalidInput     = errors.New("invalid input parameters")
    ErrNetworkError     = errors.New("network error")
    ErrServerError      = errors.New("server error")
)

// Error represents a Mobilizon API error
type Error struct {
    Code    string
    Message string
    Err     error
}

func (e *Error) Error() string {
    if e.Err != nil {
        return e.Message + ": " + e.Err.Error()
    }
    return e.Message
}

func (e *Error) Unwrap() error {
    return e.Err
}
```

### Phase 10: Add Error Handling

```go
// mobilizon/errors.go
package mobilizon

import "errors"

var (
    ErrUnauthorized     = errors.New("unauthorized: invalid or expired token")
    ErrNotFound         = errors.New("resource not found")
    ErrInvalidInput     = errors.New("invalid input parameters")
    ErrNetworkError     = errors.New("network error")
    ErrServerError      = errors.New("server error")
)

// Error represents a Mobilizon API error
type Error struct {
    Code    string
    Message string
    Err     error
}

func (e *Error) Error() string {
    if e.Err != nil {
        return e.Message + ": " + e.Err.Error()
    }
    return e.Message
}

func (e *Error) Unwrap() error {
    return e.Err
}
```

### Phase 11: Testing Strategy

#### ConcertCloud Unit Tests
```go
// concertcloud/client_test.go
package concertcloud

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
)

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

func TestClient_GetEventsByCountry(t *testing.T) {
    // Similar test for country-based queries
}
```

#### Mobilizon Unit Tests

#### Unit Tests
#### Mobilizon Unit Tests
```go
// mobilizon/auth_test.go
package mobilizon

import (
    "context"
    "testing"
)

func TestClient_Register(t *testing.T) {
    // Mock GraphQL client
    // Test registration flow
}

func TestClient_RefreshToken(t *testing.T) {
    // Mock GraphQL client
    // Test token refresh
}
```

#### Integration Tests
```go
// mobilizon_test/integration_test.go
//go:build integration
package mobilizon_test

import (
    "testing"
    "github.com/markjaroski/go-mobilizon-bot/mobilizon"
)

func TestCreateEventFlow(t *testing.T) {
    // Test against real Mobilizon instance
    // Requires test credentials
}
```

#### Mock Helper
```go
// mobilizon_test/mocks.go
package mobilizon_test

import "github.com/hasura/go-graphql-client"

type MockGraphQLClient struct {
    QueryFunc   func(ctx context.Context, q interface{}, variables map[string]interface{}) error
    MutateFunc  func(ctx context.Context, m interface{}, variables map[string]interface{}) error
}

func (m *MockGraphQLClient) Query(ctx context.Context, q interface{}, variables map[string]interface{}) error {
    if m.QueryFunc != nil {
        return m.QueryFunc(ctx, q, variables)
    }
    return nil
}

func (m *MockGraphQLClient) Mutate(ctx context.Context, mut interface{}, variables map[string]interface{}) error {
    if m.MutateFunc != nil {
        return m.MutateFunc(ctx, mut, variables)
    }
    return nil
}
```

## Implementation Steps

### Step 1: Preparation
1. Create feature branch: `git checkout -b refactor/extract-api-libraries`
2. Ensure all existing functionality works
3. Run any existing tests
4. Document current behavior

### Step 2: Add event-api Dependency
1. Add `github.com/jakopako/event-api` to go.mod
2. Run `go mod tidy`
3. Commit: "deps: add event-api dependency"

### Step 3: Create Package Structure
1. Create `mobilizon/` directory
2. Create `concertcloud/` directory
3. Create initial files: `client.go`, `types.go`, `errors.go` for both
4. Commit: "feat: create mobilizon and concertcloud package structures"

### Step 4: Extract Type Definitions
1. Move Mobilizon types to `mobilizon/types.go`
2. Create `concertcloud/types.go` using event-api models
3. Update references in bot.go
4. Commit: "refactor: extract type definitions to packages"

### Step 5: Extract ConcertCloud Client (easier, REST API)
1. Create `concertcloud/client.go`
2. Implement NewClient, GetEvents methods
3. Create tests in `concertcloud/client_test.go`
4. Commit: "refactor: extract ConcertCloud client"

### Step 6: Extract Mobilizon Authentication
1. Create `mobilizon/auth.go`
2. Move auth functions from `bot.go`
3. Update signatures to use `Client` receiver
4. Make file I/O injectable
5. Create tests in `mobilizon/auth_test.go`
6. Commit: "refactor: extract authentication to mobilizon package"

### Step 7: Extract Mobilizon Event Operations
1. Create `mobilizon/events.go`
2. Move event functions
3. Update to return structured data
4. Create tests
5. Commit: "refactor: extract event operations to mobilizon package"

### Step 8: Extract Media Operations
1. Create `mobilizon/media.go`
2. Move media functions
3. Consider splitting image processing to `internal/utils/images.go`
4. Create tests
5. Commit: "refactor: extract media operations to mobilizon package"

### Step 9: Extract Address Operations
1. Create `mobilizon/addresses.go`
2. Move address functions
3. Create tests
4. Commit: "refactor: extract address operations to mobilizon package"

### Step 10: Update bot.go
1. Import new `mobilizon` and `concertcloud` packages
2. Replace direct function calls with client methods
3. Update main() to use new APIs
4. Remove extracted code
5. Commit: "refactor: update bot.go to use new libraries"

### Step 11: Documentation
1. Add package documentation to `mobilizon/doc.go` and `concertcloud/doc.go`
2. Add README to `mobilizon/README.md` and `concertcloud/README.md`
3. Add usage examples
4. Update main README
5. Commit: "docs: add documentation for mobilizon and concertcloud packages"

### Step 12: Testing & Validation
1. Run all tests: `go test ./...`
2. Test CLI functionality manually
3. Run integration tests if available
4. Fix any issues

### Step 13: Cleanup & Polish
1. Remove unused code
2. Optimize imports
3. Run linters: `golangci-lint run`
4. Format code: `go fmt ./...`
5. Commit: "refactor: cleanup and polish"

## Benefits of This Refactoring

### Testability
- **Mockable interfaces**: Can mock GraphQL client, HTTP client, ConcertCloud API
- **No global state**: Everything passed through Client
- **Isolated units**: Each function testable independently
- **Table-driven tests**: Easy to add test cases
- **HTTP mocking**: Easy to test REST API interactions with httptest

### Maintainability
- **Clear boundaries**: Separation between CLI, Mobilizon API, and ConcertCloud API
- **Single responsibility**: Each file has focused purpose
- **Type safety**: Strong typing for parameters, using standard event-api models
- **Error handling**: Consistent error types across packages

### Reusability
- **Standalone libraries**: Can be used by other projects
- **Documented API**: Clear interfaces and examples
- **Versioned**: Can release as separate modules
- **Composable**: Mix and match functionality
- **Standard types**: Using event-api models means compatibility with other tools

### Extensibility
- **Plugin points**: Easy to add new event sources or destinations
- **Middleware**: Can add logging, metrics, retries
- **Custom clients**: Can swap HTTP/GraphQL implementations
- **Configuration**: Flexible client setup
- **Multiple data sources**: Easy to add more event sources beyond ConcertCloud

## Testing Checklist

After refactoring, verify:

**ConcertCloud Client:**
- [ ] Fetch events by city works
- [ ] Fetch events by country works
- [ ] Query parameters correctly encoded
- [ ] Pagination works
- [ ] Error handling for API failures
- [ ] HTTP timeout handling
- [ ] Response parsing works

**Mobilizon Client:**
- [ ] Registration flow works
- [ ] Authorization flow works
- [ ] Token refresh works
- [ ] Event creation works
- [ ] Event search works
- [ ] Event update works
- [ ] Media upload works
- [ ] Image download works
- [ ] Image resizing works
- [ ] Address search works
- [ ] Address creation works
- [ ] Error handling works
- [ ] Logging works
- [ ] Retry logic works

**Integration:**
- [ ] ConcertCloud → Mobilizon event sync works
- [ ] Image URLs from ConcertCloud downloaded and uploaded to Mobilizon
- [ ] Address mapping from event-api to Mobilizon works
- [ ] Event deduplication works
- [ ] All CLI flags work
- [ ] Config file loading works
- [ ] Unit tests pass
- [ ] Integration tests pass (if available)

## Migration Guide

### Before
```go
// In bot.go (global functions and inline HTTP calls)
fetchUrl := fmt.Sprintf("%s?%s", "https://api.concertcloud.live/api/events", ccQuery)
response, err := http.Get(fetchUrl)
// ... parse response

registerApp()
authorizeApp()
createMobilizonEvent(event, ...)
```

### After
```go
// Using libraries
import (
    "github.com/markjaroski/go-mobilizon-bot/mobilizon"
    "github.com/markjaroski/go-mobilizon-bot/concertcloud"
)

// Create ConcertCloud client
ccClient, err := concertcloud.NewClient(concertcloud.Config{
    Logger: logger,
})
if err != nil {
    // handle error
}

// Fetch events
events, err := ccClient.GetEventsByCity(ctx, "Lausanne", 1000)
if err != nil {
    // handle error
}

// Create Mobilizon client
mobClient, err := mobilizon.NewClient(mobilizon.Config{
    BaseURL: "https://mobilisons.ch",
    Logger:  logger,
})
if err != nil {
    // handle error
}

// Load auth
auth, err := mobilizon.LoadAuthConfig("auth.json")
if err != nil {
    // handle error
}
mobClient = mobClient.WithAuth(auth)

// Create event
for _, ccEvent := range events.Data {
    mobEvent, err := mobClient.CreateEvent(ctx, mobilizon.CreateEventParams{
        Title:       ccEvent.Title,
        Description: ccEvent.Comment,
        BeginsOn:    ccEvent.Date,
        OnlineAddress: ccEvent.URL,
        // ... other fields
    })
    if err != nil {
        // handle error
    }
}
```

## Potential Challenges

1. **Global State**: Current code uses global `gqlClient`, `auth`, `Log`
   - Solution: Pass through Client struct

2. **File I/O**: Direct file reading/writing
   - Solution: Make file operations injectable or return data

3. **User Interaction**: Prompts for device code
   - Solution: Use io.Reader/Writer interfaces for I/O

4. **HTTP Client**: Custom retry logic and error handling
   - Solution: Accept custom http.Client in config

5. **GraphQL Client**: Tightly coupled to hasura client
   - Solution: Define interface, provide default implementation

6. **Type Mapping**: Converting between ConcertCloud Event and Mobilizon Event
   - Solution: Create adapter/mapper functions in bot.go or a separate package

## Leveraging event-api Models

A key advantage of this refactoring is using the standard `github.com/jakopako/event-api/models` types for ConcertCloud events. This provides:

### Benefits
- **Compatibility**: Other tools in the ecosystem use these same types
- **Validation**: Built-in struct tags for validation
- **Documentation**: Well-documented field meanings
- **Maintenance**: Types maintained by the event-api project

### Type Usage
```go
// concertcloud/types.go
package concertcloud

import "github.com/jakopako/event-api/models"

// Event is an alias for the standard event-api Event type
type Event = models.Event

// This gives us all the fields:
// - Title, Location, City, Country
// - Date, Offset
// - URL, ImageURL, SourceURL
// - Comment, Type
// - Genres, GenresText
// - Address (with geolocation)
```

### Mapping to Mobilizon
The bot.go will need to map between ConcertCloud events and Mobilizon events:

```go
// In bot.go or a new mapper package
func concertCloudToMobilizonEvent(cc concertcloud.Event, actorID, groupID string) mobilizon.CreateEventParams {
    return mobilizon.CreateEventParams{
        Title:         cc.Title,
        Description:   formatDescription(cc),
        BeginsOn:      cc.Date,
        OnlineAddress: cc.URL,
        Category:      mapEventType(cc.Type),
        AttributedTo:  mobilizon.UUID(actorID),
        OrganizedBy:   mobilizon.UUID(groupID),
        // ... map other fields
    }
}

func formatDescription(cc concertcloud.Event) string {
    desc := cc.Comment
    if cc.GenresText != "" {
        desc += "\n\n" + cc.GenresText
    }
    if len(cc.Genres) > 0 {
        desc += "\n\nGenres: " + strings.Join(cc.Genres, ", ")
    }
    desc += "\n\n" + CC_PLUG
    return desc
}

func mapEventType(ccType string) mobilizon.EventCategory {
    switch strings.ToLower(ccType) {
    case "concert":
        return mobilizon.MUSIC
    case "theater", "theatre":
        return mobilizon.THEATRE
    case "comedy":
        return mobilizon.COMEDY
    // ... other mappings
    default:
        return mobilizon.ARTS
    }
}
```

## Future Enhancements

1. **Context Support**: Add context.Context to all functions
2. **Retry Configuration**: Make retry policy configurable
3. **Rate Limiting**: Add rate limiting for API calls
4. **Caching**: Cache address lookups, event searches
5. **Batch Operations**: Support creating multiple events
6. **Streaming**: Support streaming large responses
7. **Webhooks**: Add webhook support for real-time updates
8. **CLI Improvements**: Better error messages, progress bars
9. **Configuration**: YAML/TOML config file support
10. **Monitoring**: Add metrics, tracing support

## Timeline Estimate

- **Step 1-2** (Preparation & Dependencies): 1-2 hours
- **Step 3-4** (Package Structure & Types): 2-3 hours
- **Step 5** (ConcertCloud Client): 2-3 hours
- **Step 6** (Authentication): 2-3 hours
- **Step 7** (Events): 3-4 hours
- **Step 8** (Media): 2-3 hours
- **Step 9** (Addresses): 1-2 hours
- **Step 10** (Update bot.go): 3-4 hours
- **Step 11** (Documentation): 2-3 hours
- **Step 12** (Testing): 4-5 hours
- **Step 13** (Cleanup): 1 hour

**Total**: 23-33 hours of development time

## Conclusion

This refactoring will transform the monolithic `bot.go` into two clean, testable, and reusable libraries:

1. **`mobilizon` package** - A GraphQL-based client for Mobilizon API interactions
2. **`concertcloud` package** - A REST client for fetching events from ConcertCloud/event-api

The separation of concerns will make the code easier to maintain, test, and extend. Both libraries can be used independently by other projects:
- The `concertcloud` library can be used by any project needing to fetch events from ConcertCloud
- The `mobilizon` library can be used by any project needing to interact with Mobilizon instances
- Using standard types from `event-api` ensures compatibility with the broader ecosystem

This architecture also makes it easy to add new event sources in the future - you could create similar clients for other event APIs and pipe them into Mobilizon using the same patterns.
