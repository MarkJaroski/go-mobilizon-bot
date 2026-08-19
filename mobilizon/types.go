package mobilizon

import (
	"time"

	"github.com/google/uuid"
)

// AuthConfig holds OAuth2 tokens
type AuthConfig struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

// EventParams holds parameters for creating an event
type EventParams struct {
	UUID                     *uuid.UUID
	Title                    string
	Description              string
	BeginsOn                 time.Time
	EndsOn                   time.Time
	OrganizerActorId         int
	AttributedToId           int
	Category                 EventCategory
	Visibility               EventVisibility
	JoinOptions              EventJoinOptions
	PhysicalAddr             *AddressInput
	OnlineAddress            string
	ExternalParticipationURL string
	Draft                    bool
	PhysicalAddress          AddressInput
	Status                   EventStatus
	Tags                     []*string
	Options                  EventOptionsInput
	ImageURL                 string
	Contact                  []*Contact
	AttributedTo             uuid.UUID
	OrganizedBy              uuid.UUID
}

type Event struct {
	ID            string
	UUID          uuid.UUID
	Title         string
	Description   string
	BeginsOn      time.Time
	EndsOn        time.Time
	OnlineAddress string
}

type UploadMediaResponse struct {
	Data struct {
		UploadMedia struct {
			UUID uuid.UUID `json:"uuid"`
		} `json:"uploadMedia"`
	} `json:"data"`
	Errors []MediaUploadError `json:"errors"`
}

type MediaUploadError struct {
	Code      string   `json:"code"`
	Field     string   `json:"string"`
	Message   string   `json:"message"`
	Path      []string `json:"path"`
	Locations []struct {
		Column int `json:"column"`
		Line   int `json:"line"`
	} `json:"locations"`
	StatusCode int `json:"status_code"`
}
