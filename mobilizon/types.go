package mobilizon

import (
	"time"

	"github.com/google/uuid"
)

// CreateEventParams holds parameters for creating an event
type CreateEventParams struct {
	Title            string
	Description      string
	BeginsOn         time.Time
	EndsOn           time.Time
	Category         EventCategory
	Visibility       EventVisibility
	JoinOptions      EventJoinOptions
	AttributedTo     uuid.UUID
	OrganizedBy      uuid.UUID
	PhysicalAddr     *AddressInput
	OnlineAddress    string
	Draft            bool
	OrganizerActorId int
	AttributedToId   int
	PhysicalAddress  AddressInput
	Status           EventStatus
	Tags             []string
	Options          EventOptionsInput
	Contact          []Contact
	Metadata         []EventMetadataInput
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
