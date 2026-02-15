package mobilizon

import (
	"context"
	"time"

	"github.com/hasura/go-graphql-client"
)

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

// CreateEventParams holds parameters for creating an event
type UpdateEventParams struct {
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

// EventOptionsInput represents the cooresponding Mobilizòn GraphQL type
type EventOptionsInput struct {
	CommentModeration EventCommentModeration `json:"commentModeration"`
	ShowStartTime     graphql.Boolean        `json:"showStartTime"`
	ShowEndTime       graphql.Boolean        `json:"showEndTime"`
	Timezone          Timezone               `json:"timezone"`
}

// EventCategory represents the list of possible event categories present
// in Mobilizòn. Obviously this list must be maintained here as the list in
// the Mobilizòn codebase changes.
type EventCategory string

const (
	ARTS                          EventCategory = "ARTS"
	AUTO_BOAT_AIR                 EventCategory = "AUTO_BOAT_AIR"
	BOOK_CLUBS                    EventCategory = "BOOK_CLUBS"
	BUSINESS                      EventCategory = "BUSINESS"
	CAUSES                        EventCategory = "CAUSES"
	COMEDY                        EventCategory = "COMEDY"
	COMMUNITY                     EventCategory = "COMMUNITY"
	CRAFTS                        EventCategory = "CRAFTS"
	FAMILY_EDUCATION              EventCategory = "FAMILY_EDUCATION"
	FASHION_BEAUTY                EventCategory = "FASHION_BEAUTY"
	FILM_MEDIA                    EventCategory = "FILM_MEDIA"
	FOOD_DRINK                    EventCategory = "FOOD_DRINK"
	GAMES                         EventCategory = "GAMES"
	HEALTH                        EventCategory = "HEALTH"
	LANGUAGE_CULTURE              EventCategory = "LANGUAGE_CULTURE"
	LEARNING                      EventCategory = "LEARNING"
	LGBTQ                         EventCategory = "LGBTQ"
	MEETING                       EventCategory = "MEETING"
	MOVEMENTS_POLITICS            EventCategory = "MOVEMENTS_POLITICS"
	MUSIC                         EventCategory = "MUSIC"
	NETWORKING                    EventCategory = "NETWORKING"
	OUTDOORS_ADVENTURE            EventCategory = "OUTDOORS_ADVENTURE"
	PARTY                         EventCategory = "PARTY"
	PERFORMING_VISUAL_ARTS        EventCategory = "PERFORMING_VISUAL_ARTS"
	PETS                          EventCategory = "PETS"
	PHOTOGRAPHY                   EventCategory = "PHOTOGRAPHY"
	SCIENCE_TECH                  EventCategory = "SCIENCE_TECH"
	SPIRITUALITY_RELIGION_BELIEFS EventCategory = "SPIRITUALITY_RELIGION_BELIEFS"
	SPORTS                        EventCategory = "SPORTS"
	THEATRE                       EventCategory = "THEATRE"
)

var EventCategoryStrings = []string{
	"ARTS",
	"AUTO_BOAT_AIR",
	"BOOK_CLUBS",
	"BUSINESS",
	"CAUSES",
	"COMEDY",
	"COMMUNITY",
	"CRAFTS",
	"FAMILY_EDUCATION",
	"FASHION_BEAUTY",
	"FILM_MEDIA",
	"FOOD_DRINK",
	"GAMES",
	"HEALTH",
	"LANGUAGE_CULTURE",
	"LEARNING",
	"LGBTQ",
	"MEETING",
	"MOVEMENTS_POLITICS",
	"MUSIC",
	"NETWORKING",
	"OUTDOORS_ADVENTURE",
	"PARTY",
	"PERFORMING_VISUAL_ARTS",
	"PETS",
	"PHOTOGRAPHY",
	"SCIENCE_TECH",
	"SPIRITUALITY_RELIGION_BELIEFS",
	"SPORTS",
	"THEATRE",
}

// EventVisibility represents the EventVisibility Mobilizòn GraphQL type
type EventVisibility string

const (
	PRIVATE    EventVisibility = "PRIVATE"
	PUBLIC     EventVisibility = "PUBLIC"
	RESTRICTED EventVisibility = "RESTRICTED"
	UNLISTED   EventVisibility = "UNLISTED"
)

// EventJoinOptions represents the EventJoinOptions Mobilizòn GraphQL type
type EventJoinOptions string

const (
	FREE     EventJoinOptions = "FREE"
	EXTERNAL EventJoinOptions = "EXTERNAL"
)

// DateTime represents the DateTime Mobilizòn GraphQL type
type DateTime string

// EventCommentModeration represents the EventCommentModeration Mobilizòn
// GraphQL type
type EventCommentModeration string

const (
	ALLOW_ALL EventCommentModeration = "ALLOW_ALL"
	CLOSED    EventCommentModeration = "CLOSED"
	MODERATED EventCommentModeration = "MODERATED"
)

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

// Point represents the latitude and longitude of a place in Mobilizòn
type Point string

// AddressInput represents address data in Mobilizòn GraphQL mutations like
// createEvent and updateEvent
type AddressInput struct {
	Id          int    `json:"id"`
	Description string `json:"description"`
	Locality    string `json:"locality"`
	PostalCode  string `json:"postalCode"`
	Street      string `json:"street"`
	Country     string `json:"country"`
	Region      string `json:"region"`
	Geom        Point  `json:"geom"`
}

// MediaUpload represents the GraphQL MediaUpload type
type MediaUpload struct {
	Uuid UUID `json:"uuid"`
}

// MediaInput represents media data in Mobilizòn GraphQL mutations like
// createEvent and updateEvent
type MediaInput struct {
	MediaUuid UUID `json:"mediaUuid"`
}

// MediaData represents the mediaUpload object of a GraphQL mediaUpload mutation
type MediaData struct {
	Upload MediaUpload `json:"uploadMedia"`
}

// MediaData represents the response object of a GraphQL mediaUpload mutation
type MediaResponse struct {
	Data MediaData `json:"data"`
}

// Timezone represents the cooresponding Mobilizòn GraphQL type
type Timezone string

// AuthConfig is the OAuth2 response presented by Mobilizòn for
// authorization and reauthorization. Becomes the structure of the auth
type AuthConfig struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Scopes                string `json:"scopes"`
	TokenType             string `json:"token_type"`
}

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
