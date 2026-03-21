// Package mobilizon implements a Mobilizon GraphQL client for golang
package mobilizon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type RegisterConfig struct {
	BaseURL string
	AppName string
	Website string
	Scopes  []string
}

type Registration struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Name         string `json:"name"`
}

// RegisterApp registers a new OAuth2 application with a Mobilizon instance
// This is a one-time setup operation typically run before first use
func RegisterApp(ctx context.Context, baseURL, appName, website string, scopes []string) (*Registration, error) {
	if scopes == nil {
		scopes = DefaultScopes()
	}

	params := url.Values{}
	params.Set("name", appName)
	params.Set("scopes", strings.Join(scopes, " "))
	params.Set("website", website)
	params.Set("redirect_uri", "https://example.com/endpoint") // Unused for device flow

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		baseURL+"/apps",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registration failed (status %d): %s", resp.StatusCode, body)
	}

	var reg Registration
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, fmt.Errorf("failed to parse registration response: %w", err)
	}

	return &reg, nil
}

// DefaultScopes returns the default OAuth2 scopes for Mobilizon
func DefaultScopes() []string {
	return []string{
		"write:event:create",
		"write:event:update",
		"write:media:upload",
	}
}

// SaveRegistration saves the registration to a file
func SaveRegistration(filepath string, reg *Registration) error {
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0600)
}

// LoadRegistration loads a registration from a file
func LoadRegistration(filepath string) (*Registration, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	var reg Registration
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, err
	}

	return &reg, nil
}
