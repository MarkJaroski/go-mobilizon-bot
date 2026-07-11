// Package mobilizon implements a Mobilizon GraphQL client for golang
package mobilizon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/oauth2"
)

// required because our server seems to crash once in a while
const SERVER_CRASH_WAIT_TIME = time.Duration(1 * int64(time.Minute))

// Client wraps the genqlient GraphQL client
type Client struct {
	baseURL      string
	clientID     string
	oauth2Config *oauth2.Config
	token        *oauth2.Token
	gqlClient    graphql.Client
}

// NewClient creates a new Mobilizon client
func NewClient(baseURL string, clientID string) (*Client, error) {
	if clientID == "" {
		return nil, errors.New("clientID is required - call mobilizon.RegisterApp() first")
	}

	return &Client{
		baseURL:   baseURL,
		clientID:  clientID,
		gqlClient: graphql.NewClient(baseURL+"/api", http.DefaultClient),
		oauth2Config: &oauth2.Config{
			ClientID: clientID,
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
	}, nil
}

// initGraphQLClient initializes the GraphQL client with auth
func (c *Client) initGraphQLClient(ctx context.Context) {
	httpClient := c.oauth2Config.Client(ctx, c.token)
	c.gqlClient = graphql.NewClient(c.baseURL+"/api", httpClient)
}

// performs the OAuth2 handshake to obtain an account holder's authorization
// and then initializes the client with the refresh token
//
// This also handles token renewal
func (c *Client) EnsureAuthorization(ctx context.Context, tokenPath string) error {
	if err := c.LoadToken(tokenPath); err == nil {
		if err := c.RefreshToken(ctx); err == nil {
			c.SaveToken(tokenPath)
			c.initGraphQLClient(ctx)
			return nil
		}
	}
	if err := c.Authorize(ctx); err != nil {
		return err
	}
	c.SaveToken(tokenPath)
	c.initGraphQLClient(ctx)
	return nil
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

	// set up an HTTPClient with automated retries
	retryClient := retryablehttp.NewClient()
	retryClient.RetryWaitMin = SERVER_CRASH_WAIT_TIME
	retryClient.RetryWaitMax = time.Duration(10 * int64(time.Minute))
	retryClient.RetryMax = 120
	retryClient.CheckRetry = RetryPolicy
	retryClient.Backoff = ErrorBackoff

	httpClient := c.oauth2Config.Client(ctx, c.token)
	retryClient.HTTPClient = httpClient
	c.gqlClient = graphql.NewClient(c.baseURL+"/api", retryClient.StandardClient())

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

func (c *Client) RefreshToken(ctx context.Context) error {
	resp, err := RefreshAuthTokens(ctx, c.gqlClient, c.token.RefreshToken)
	if err != nil {
		return err
	}
	c.token = &oauth2.Token{
		AccessToken:  resp.RefreshToken.AccessToken,
		RefreshToken: resp.RefreshToken.RefreshToken,
		Expiry:       time.Now().Add(time.Hour * 8),
	}
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
	// Load the file
	fileContents, fi, err := loadFileContents(filepath)
	if err != nil {
		return nil, err
	}

	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// TODO make this a template string or something to avoid the long line
	writer.WriteField("query", "mutation uploadMedia($file: Upload!, $name: String!) { uploadMedia(file: $file, name: $name) { uuid } }")
	writer.WriteField("variables", "{\"name\":\""+fi.Name()+"\",\"file\":\"image1\"}")

	part, err := writer.CreateFormFile("image1", fi.Name())
	if err != nil {
		return nil, err
	}
	part.Write(fileContents)
	writer.Close()

	r, err := http.NewRequest("POST", c.baseURL+"/api", body)
	if err != nil {
		return nil, err
	}
	r.Header.Add("Content-Type", writer.FormDataContentType())
	r.Header.Add("Authorization", "Bearer "+c.token.AccessToken)

	resp, err := c.HTTPClient(ctx).Do(r)
	if err != nil {
		return nil, err
	}

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var respJSON UploadMediaResponse
	json.Unmarshal(respData, &respJSON)
	return &respJSON.Data.UploadMedia.UUID, nil

}

// CreateEvent creates an event
func (c *Client) CreateEvent(
	ctx context.Context,
	params EventParams,
) (*uuid.UUID, error) {

	var picture *MediaInput = nil

	if params.ImageURL != "" {
		var mi MediaInput
		if path, err := downloadFile(params.ImageURL); err == nil {
			if uuid, err := c.UploadMediaFile(ctx, path); err == nil {
				mi.MediaUuid = uuid
				picture = &mi
			} else {
				return nil, errors.New("Error uploading image: " + path)
			}
		} else {
			return nil, errors.New("Error Downloading image: " + params.ImageURL)
		}
	}

	endDate := strconv.Itoa(params.AttributedToId)

	// Create the event with the media UUID
	resp, err := CreateEvent(
		ctx,
		c.gqlClient,
		strconv.Itoa(params.OrganizerActorId),
		&endDate,
		params.Title,
		params.Description,
		params.BeginsOn,
		&params.EndsOn,
		&params.Status,
		&params.Visibility,
		&params.JoinOptions,
		&params.ExternalParticipationURL,
		&params.Draft,
		params.Tags,
		picture,
		&params.OnlineAddress,
		&params.Category,
		&params.PhysicalAddress,
		&params.Options,
		params.Contact,
	)
	if err != nil {
		return nil, err
	}

	return resp.CreateEvent.Uuid, nil
}

// UpdateEvent updates an event
func (c *Client) UpdateEvent(
	ctx context.Context,
	params EventParams,
) (*uuid.UUID, error) {

	var picture *MediaInput = nil

	if params.ImageURL != "" {
		var mi MediaInput
		if path, err := downloadFile(params.ImageURL); err == nil {
			if uuid, err := c.UploadMediaFile(ctx, path); err == nil {
				mi.MediaUuid = uuid
				picture = &mi
			}
		}
	}

	// get the existing event ID using the UUID
	fre, err := FetchEvent(ctx, c.gqlClient, *params.UUID)
	if err != nil {
		return nil, err
	}

	organizer := strconv.Itoa(params.OrganizerActorId)
	attribute := strconv.Itoa(params.AttributedToId)

	// Update the event with the media UUID
	resp, err := UpdateEvent(
		ctx,
		c.gqlClient,
		*fre.Event.FullEvent.Id,
		&params.Title,
		&params.Description,
		&params.BeginsOn,
		&params.EndsOn,
		&params.Status,
		&params.Visibility,
		&params.JoinOptions,
		&params.ExternalParticipationURL,
		&params.Draft,
		params.Tags,
		picture,
		&params.OnlineAddress,
		&organizer,
		&attribute,
		&params.Category,
		&params.PhysicalAddress,
		&params.Options,
		params.Contact,
	)
	if err != nil {
		return nil, err
	}

	return resp.UpdateEvent.Uuid, nil
}

// // CreateOrUpdateEvent creates an event if params.UUID is nil, otherwise updates it
// func (c *Client) CreateOrUpdateEvent(ctx context.Context, params EventParams) (*uuid.UUID, error) {
// 	if params.UUID != nil {
// 		return c.UpdateEvent(ctx, params)
// 	}
// 	return c.CreateEvent(ctx, params)
// }

// SearchForEvents searches for events by term
func (c *Client) SearchForEvents(ctx context.Context, term string, beginsOn time.Time) ([]Event, error) {
	resp, err := SearchEvents(ctx, c.gqlClient, &term, &beginsOn)
	if err != nil {
		return nil, err
	}

	events := make([]Event, len(resp.SearchEvents.Elements))
	for i, elem := range resp.SearchEvents.Elements {
		events[i] = Event{
			ID:       *elem.Id,
			UUID:     *elem.Uuid,
			Title:    *elem.Title,
			BeginsOn: *elem.BeginsOn,
		}
	}

	return events, nil
}

func (c *Client) EventExists(ctx context.Context, title string, location string, beginsOn time.Time) (bool, *uuid.UUID, error) {
	resp, err := SearchEvents(ctx, c.gqlClient, &title, &beginsOn)
	if err != nil {
		return false, nil, err
	}

	// no result, the event does not exist
	if resp.SearchEvents.Total == 0 {
		return false, nil, nil
	}

	// get the list of events found
	elems := resp.SearchEvents.Elements

	id := elems[0].Uuid

	if *id == uuid.Nil {
		return false, nil, errors.New("Empty UUID returned from SearchEvents")
	}

	// choosing between multiple matching events is going to be very
	// difficult, and the first one will always be the best match for the
	// date so that's the one we'll return
	return true, id, nil
}

func (c *Client) FetchAddr(ctx context.Context, query string) ([]AddressInput, error) {
	resp, err := SearchAddress(ctx, c.gqlClient, query)
	if err != nil {
		return nil, err
	}

	addrs := make([]AddressInput, len(resp.SearchAddress))

	for i, elem := range resp.SearchAddress {
		fragment := elem.AdressFragment
		addrs[i] = AddressInput{
			Id:          fragment.Id,
			Geom:        fragment.Geom,
			Street:      fragment.Street,
			Locality:    fragment.Locality,
			PostalCode:  fragment.PostalCode,
			Region:      fragment.Region,
			Country:     fragment.Country,
			Description: fragment.Description,
			Type:        fragment.Type,
			Url:         fragment.Url,
			OriginId:    fragment.OriginId,
			Timezone:    fragment.Timezone,
		}
	}

	return addrs, nil
}

func (c *Client) FetchEvent(uuid.UUID) (*Event, error) {
	return nil, errors.New("FetchEvents() not implemented.")
}

// mobilizònRetryPolicy impements the RetryPolicy interface from
// hashicorp.retryablehttp, which captures the main failure modes cause by
// an ephemeral crash of the Mobilizòn server process
func RetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if resp.Status == "401" {
		return true, nil
	}
	if resp.Status < "400" {
		return false, nil
	}
	if resp.Status >= "405" {
		return true, nil
	}
	return false, nil
}

// mobilizònErrorBackoff implements the Backoff interface from
// hashicorp.retryablehttp, waiting long enough for Mobilizòn to recover
// from an activity-pub related crash
func ErrorBackoff(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
	return SERVER_CRASH_WAIT_TIME
}
