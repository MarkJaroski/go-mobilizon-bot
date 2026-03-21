// Package mobilizon implements a Mobilizon GraphQL client for golang
package mobilizon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
	"golang.org/x/oauth2"
)

// Client wraps the genqlient GraphQL client
type Client struct {
	name         string
	baseURL      string
	url          string
	log          hclog.Logger
	oauth2Config *oauth2.Config
	token        *oauth2.Token
	gqlClient    graphql.Client
}

// AuthConfig holds OAuth2 tokens
type AuthConfig struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

// NewClient creates a new Mobilizon client
func NewClient(baseURL string, log hclog.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		log:     log,
		oauth2Config: &oauth2.Config{
			ClientID: "",
			Scopes: []string{
				"write:event:create",
				"write:event:update",
				"write:media:upload",
			},
			Endpoint: oauth2.Endpoint{
				AuthURL:       baseURL + "/oauth/authorize",
				TokenURL:      baseURL + "/oauth/token",
				DeviceAuthURL: baseURL + "/login/device/code",
			},
		},
	}
}

// performs the OAuth2 device code flow to authorize our graphql client
func (c *Client) Authorize(ctx context.Context) error {
	if c.oauth2Config.ClientID == "" {
		return fmt.Errorf("The OAuth2Config has no client ID. Call Register() or LoadClientID()")
	}

	// get the device code
	deviceAuth, err := c.oauth2Config.DeviceAuth(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device code: %w", err)
	}

	// Display instructions to user
	fmt.Printf("\n=== Device Authorization ===\n")
	fmt.Printf("Visit: %s\n", deviceAuth.VerificationURI)
	fmt.Printf("Enter code: %s\n", deviceAuth.UserCode)
	fmt.Printf("============================\n\n")

	// Wait for user to authorize (or use automatic polling)
	fmt.Println("Waiting for authorization...")

	// Poll for access token
	token, err := c.oauth2Config.DeviceAccessToken(ctx, deviceAuth)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	c.token = token

	httpClient := c.oauth2Config.Client(ctx, c.token)
	c.gqlClient = graphql.NewClient(c.baseURL+"/api", httpClient)

	return nil
}

// registers our client as an application with the mobilizon instance
func (c *Client) Register(ctx context.Context) error {
	var posturl = c.baseURL + "/apps"

	// FIXME build this with the name and scope from the client settings
	params := "name=" + url.QueryEscape(c.name)
	params += "&scope=" + strings.Join(c.oauth2Config.Scopes, "%20")
	params += "&website=" + c.url
	// the device code flow doesn't use a redirect_uri so we can put anything here
	params += "&redirect_uri=https://example.com/endpoint"
	body := []byte(params)

	r, err := http.NewRequest("POST", posturl, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.HTTPClient(ctx).Do(r)
	if err != nil {
		return err
	}

	resData, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	var reg Registration
	json.Unmarshal(resData, &reg)

	c.oauth2Config.ClientID = reg.ClientID

	return nil
}

// stores the OAuth2 Client ID
func (c *Client) SaveClientID(filepath string) error {
	if c.oauth2Config.ClientID == "" {
		return fmt.Errorf("no clientID to save")
	}

	return os.WriteFile(filepath, []byte(c.oauth2Config.ClientID), 0600)
}

// loads the OAuth2 Client ID
func (c *Client) LoadClientID(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	c.oauth2Config.ClientID = string(data)

	return nil
}

// SaveToken saves the token to a file
func (c *Client) SaveToken(filepath string) error {
	if c.token == nil {
		return fmt.Errorf("no token to save")
	}

	data, err := json.MarshalIndent(c.token, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, data, 0600)
}

// LoadToken loads a token from a file
func (c *Client) LoadToken(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return err
	}

	c.token = &token
	return nil
}

// GraphQLClient returns the underlying GraphQL client for direct use
// This allows advanced users to call genqlient functions directly
func (c *Client) GraphQLClient() graphql.Client {
	return c.gqlClient
}

// HTTPClient returns an authenticated HTTP client
// Useful for other HTTP operations beyond GraphQL
func (c *Client) HTTPClient(ctx context.Context) *http.Client {
	return c.oauth2Config.Client(ctx, c.token)
}

// UploadMediaFile uploads a file and returns the media UUID
func (c *Client) UploadMediaFile(ctx context.Context, filepath string) (*uuid.UUID, error) {
	// Open the file
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Use generated UploadMedia function
	resp, err := UploadMedia(ctx, c.gqlClient,
		graphql.Upload{
			FileName: filepath,
			Body:     file,
		},
		filepath,
	)
	if err != nil {
		return nil, err
	}

	return &resp.UploadMedia.Uuid, nil
}

// CreateEventWithMedia creates an event with an image
func (c *Client) CreateEventWithMedia(
	ctx context.Context,
	params CreateEventParams,
	imagePath string,
) (*uuid.UUID, error) {
	// First upload the media
	mediaUUID, err := c.UploadMediaFile(ctx, imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to upload media: %w", err)
	}

	// Create the event with the media UUID
	resp, err := createEvent(
		ctx,
		c.gqlClient,
		strconv.Itoa(params.OrganizerActorId),
		strconv.Itoa(params.AttributedToId),
		params.Title,
		params.Description,
		params.BeginsOn,
		params.EndsOn,
		params.Status,
		params.Visibility,
		params.JoinOptions,
		params.OnlineAddress,
		params.Draft,
		params.Tags,
		MediaInput{MediaUuid: *mediaUUID}, // Use uploaded media
		params.OnlineAddress,
		params.Category,
		params.PhysicalAddress,
		params.Options,
		params.Contact,
		params.Metadata,
	)
	if err != nil {
		return nil, err
	}

	return &resp.CreateEvent.Uuid, nil
}

// SearchForEvents searches for events by term
func (c *Client) SearchForEvents(ctx context.Context, term string, beginsOn time.Time) ([]Event, error) {
	resp, err := SearchEvents(ctx, c.gqlClient, term, beginsOn)
	if err != nil {
		return nil, err
	}

	events := make([]Event, len(resp.SearchEvents.Elements))
	for i, elem := range resp.SearchEvents.Elements {
		events[i] = Event{
			ID:            elem.Id,
			UUID:          elem.Uuid,
			Title:         elem.Title,
			BeginsOn:      elem.BeginsOn,
			OnlineAddress: elem.OnlineAddress,
		}
	}

	return events, nil
}
