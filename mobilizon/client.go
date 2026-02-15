package mobilizon

import (
	"net/http"

	"github.com/hashicorp/go-hclog"
	"github.com/hasura/go-graphql-client"
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
