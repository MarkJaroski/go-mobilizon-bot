package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/signal"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"

	"github.com/markjaroski/go-mobilizon-bot/concertcloud"
	"github.com/markjaroski/go-mobilizon-bot/mobilizon"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/pflag"
)

const CC_PLUG = "Help promote your favourite venues with: https://concertcloud.live/contribute"
const ADDR_FILE = "addrs.json"
const EXISTS_FILE = "exists.json"

// Options represents the full set of command-line options for the bot
type Options struct {
	MobilizonUrl *string
	City         *string
	Country      *string
	Limit        *int
	Page         *int
	Radius       *int
	Date         *string
	File         *string
	AuthConfig   *string
	Config       *string
	ActorID      *int
	GroupID      *int
	Timezone     *string
	NoOp         *bool
	Register     *bool
	Authorize    *bool
	Draft        *bool
	Debug        *bool
	AddrsFile    *string
	ExistsFile   *string
	AppName      *string
	AppURL       *string
}

type ExistingEvent struct {
	UUID  uuid.UUID          `json:"uuid"`
	Event concertcloud.Event `json:"event"`
}

var opts Options

// local fields
var mobClient *mobilizon.Client
var auth mobilizon.AuthConfig
var actorID int
var groupID int
var timezone *string
var addrs map[string]mobilizon.AddressInput
var existing map[string]ExistingEvent
var created map[string]ExistingEvent
var addrsFile string
var existsFile string
var authFile string
var registration *mobilizon.Registration

// Log is our hclog local instance
var Log hclog.Logger

// init sets up logging and initializes the addr map
func init() {
	Log = hclog.New(&hclog.LoggerOptions{
		Name:  "Mobilizon bot",
		Level: hclog.LevelFromString("INFO"),
	})
	addrs = make(map[string]mobilizon.AddressInput)
	existing = make(map[string]ExistingEvent)
	created = make(map[string]ExistingEvent)
}

// FIXME: main still does too much of the work
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// set up our config dir if it's not already there
	confdir, err := os.UserConfigDir()
	if err != nil {
		Log.Error("User config dir not found", err)
		os.Exit(1)
	}
	err = os.Mkdir(confdir+"/mobilizon", 0700)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		Log.Error("Error creating directory", err)
		os.Exit(1)
	}

	opts.MobilizonUrl = pflag.String("mobilizonurl", "https://mobilisons.ch", "Your Mobilizon base URL")
	opts.AppName = pflag.String("appname", "Concert Cloud", "The name of your client app")
	opts.AppURL = pflag.String("appurl", "https://concertcloud.live", "Your client app's about page")
	opts.City = pflag.String("city", "", "The concertcloud API param 'city'")
	opts.Country = pflag.String("country", "", "The concertcloud API param 'country'")
	opts.Limit = pflag.Int("limit", 10, "The concertcloud API param 'limit'")
	opts.Page = pflag.Int("page", 0, "The concertcloud API param 'page'")
	opts.Radius = pflag.Int("radius", 25, "The concertcloud API param 'radius'")
	opts.Date = pflag.String("date", "", "The concertcloud API param 'date'")
	opts.File = pflag.String("file", "", "Instead of fetching from concertcloud, use local file.")
	opts.ActorID = pflag.Int("actor", -1, "The Mobilizon actor ID to use as the event organizer.")
	opts.GroupID = pflag.Int("group", -1, "The Mobilizon group ID to use for the event attribution.")
	opts.Timezone = pflag.String("timezone", "Europe/Zurich", "The timezone to use for the event attribution.")
	opts.AuthConfig = pflag.String("authconfig", confdir+"/mobilizon/auth.json", "Use this file for authorization tokens.")
	opts.Config = pflag.String("config", confdir+"/mobilizon", "Use this directory for configuration.")
	opts.NoOp = pflag.Bool("noop", false, "Gather all required information and report on it, but do not create events in Mobilizòn.")
	opts.Register = pflag.Bool("register", false, "Register this bot and quit. A client id will be output.")
	opts.Authorize = pflag.Bool("authorize", false, "Authorize this bot and quit. An auth token and renew token will be output.")
	opts.Draft = pflag.Bool("draft", false, "Create events in draft mode.")
	opts.Debug = pflag.Bool("debug", false, "Debug mode.")

	pflag.Parse()

	if *opts.Debug {
		Log.SetLevel(hclog.LevelFromString("DEBUG"))
	}

	if *opts.Register {
		conf := mobilizon.RegisterConfig{
			BaseURL: *opts.MobilizonUrl,
			AppName: *opts.AppName,
			Website: *opts.AppURL,
			Scopes:  mobilizon.DefaultScopes(),
		}
		registration, err = mobilizon.RegisterApp(ctx, conf)
		if err != nil {
			Log.Error("error", err)
			os.Exit(1)
		}
		mobilizon.SaveRegistration(*opts.Config+"/registration.json", registration)
		// register is a one-off activity
		return
	}

	if registration == nil {
		registration, err = mobilizon.LoadRegistration(*opts.Config + "/registration.json")
		if err != nil {
			panic("No registration found for application " + *opts.AppName)
		}
		*opts.MobilizonUrl = registration.BaseURL
	}

	if *opts.Config != confdir+"/mobilizon" {
		*opts.AuthConfig = *opts.Config + "/auth.json"
	}

	mobClient, err = mobilizon.NewClient(*opts.MobilizonUrl, registration.ClientID)
	if err != nil {
		Log.Error("Error creating client", err)
		panic("Unable to create mobilizon client")
	}

	// do the authorization
	if err = mobClient.EnsureAuthorization(ctx, *opts.AuthConfig); err != nil {
		Log.Error("error", err)
		panic(*opts.AppName + " not Authorized")
	}

	// if the user has asked to stop at authorization we're done
	if *opts.Authorize {
		return
	}

	actorID = *opts.ActorID
	groupID = *opts.GroupID

	addrsFile = *opts.Config + "/" + ADDR_FILE
	existsFile = *opts.Config + "/" + EXISTS_FILE

	// this will hold our json object whether local or from ConcertCloud
	var events []concertcloud.Event

	if *opts.File != "" {
		Log.Info("using local file:", "file", *opts.File)
		dat, err := os.ReadFile(*opts.File)
		if err != nil {
			Log.Error("error", err)
			os.Exit(1)
		}
		// goskyr file output produces a simple json array of Event objects
		json.Unmarshal(dat, &events)
	} else {
		ccConfig := concertcloud.Config{
			Logger:     Log,
			HTTPClient: mobClient.HTTPClient(ctx),
		}
		ccClient, err := concertcloud.NewClient(ccConfig)
		params := concertcloud.QueryParams{
			City:    *opts.City,
			Country: *opts.Country,
			Limit:   *opts.Limit,
			Page:    *opts.Page,
			Radius:  *opts.Radius,
			Date:    *opts.Date,
		}
		resp, err := ccClient.GetEvents(ctx, params)
		// Fetch some concerts from Concert Cloud
		if err != nil {
			Log.Error("error", err)
		}

		events = resp.Data
	}

	// fetchAddrs(ctx, events)
	createEvents(ctx, events)
}

func loadAddresses() {
	dat, err := os.ReadFile(addrsFile)
	if err != nil {
		Log.Error(err.Error())
		return
	}
	err = json.Unmarshal(dat, &addrs)
	if err != nil {
		Log.Error(err.Error())
	}
}

func saveAddresses() {
	Log.Debug("Saving addresses", "file", addrsFile)
	data, err := json.MarshalIndent(&addrs, "", " ")
	if err != nil {
		Log.Error(err.Error())
	}
	err = os.WriteFile(addrsFile, data, 0600)
	if err != nil {
		Log.Error(err.Error())
	}
}

func loadExistingEvents() {
	dat, err := os.ReadFile(existsFile)
	if err != nil {
		Log.Error(err.Error())
		return
	}
	err = json.Unmarshal(dat, &existing)
	if err != nil {
		Log.Error(err.Error())
	}
}

func saveExistingEvents() {
	Log.Debug("Saving existing events", "file", existsFile)
	data, err := json.MarshalIndent(&created, "", " ")
	if err != nil {
		Log.Error(err.Error())
	}
	err = os.WriteFile(existsFile, data, 0600)
	if err != nil {
		Log.Error(err.Error())
	}
}

// Create a hopefully unique key for a given event.
//
// The lineup may change, and many venues change the title to indicate
// that an event is cancelled, so we can't use that.
//
// A previous version used the URL, but some venues change that as well
func eventKey(e concertcloud.Event) string {
	return e.City + "/" + e.Location + "/" + e.Date.Format(time.RFC3339)
}

// Create a hopefully unique key for a given address
func addrKey(e concertcloud.Event) string {
	return e.City + "/" + e.Location
}

// Convert the concert cloud Address from an event into AddressInput for Mobilizon
func addressToAddressInput(e concertcloud.Event) mobilizon.AddressInput {
	geo := e.Address.Geolocacation.MongoGeolocation.Coordinates
	// offset := time.Duration(e.Offset * int(time.Second))
	// tzName := "UTC" + fmt.Sprintf("%+.0f", offset.Hours())
	latlong := strconv.FormatFloat(geo[0], 'f', 8, 64) + ";" + strconv.FormatFloat(geo[1], 'f', 8, 64)
	street := e.Address.HouseNumber + " " + e.Address.Street
	originId := "nominatim:" + strconv.FormatInt(e.Address.Geolocacation.OsmID, 10)
	return mobilizon.AddressInput{
		Geom:        &latlong,
		Street:      &street,
		Locality:    &e.Address.Locality,
		PostalCode:  &e.Address.PostCode,
		Region:      &e.Address.State,
		Country:     &e.Address.Country,
		Description: &e.Location,
		OriginId:    &originId,
		/* Url:         &e.SourceURL, */
	}
}

// createEvents loops through all of the events in the json input, sets up
// their variables map, and runs createEvents on them
func createEvents(ctx context.Context, events []concertcloud.Event) {

	Log.Debug("createEvents()", "number of events: ", len(events))

	loadExistingEvents()

	for _, e := range events {

		// Do not upload events from bejazz.ch. They don't like us.
		//
		// FIXME: this should be loaded from an opt out file or something
		if match, _ := regexp.MatchString("bejazz.ch", e.URL); match {
			Log.Info("Skipping BeJazz.")
			continue
		}

		// NoOp calls for a dry run
		if *opts.NoOp {
			continue
		}

		// Log a warning for missing venues and skip
		if e.Address.Street == "" {
			Log.Info("Address not found", "location", e.Location, "city", e.City)
			continue
		}

		// trim the title to produce better matches
		e.Title = strings.TrimSpace(e.Title)

		// titles must be at least 3 characters long in Mobilizòn so we
		// have to pad the really short ones
		if utf8.RuneCountInString(e.Title) < 3 {
			e.Title = e.Title + " ..."
		}

		vars := mobilizon.EventParams{
			Title:                    e.Title,
			Description:              e.Comment + " <p/><p> " + CC_PLUG,
			BeginsOn:                 e.Date,
			EndsOn:                   e.Date.Add(time.Hour * 2),
			Category:                 populateCategory(e),
			Visibility:               mobilizon.EventVisibilityPublic,
			JoinOptions:              mobilizon.EventJoinOptionsExternal,
			PhysicalAddress:          addressToAddressInput(e),
			OnlineAddress:            e.URL,
			ExternalParticipationURL: e.URL,
			Draft:                    *opts.Draft,
			OrganizerActorId:         actorID,
			AttributedToId:           groupID,
			Tags:                     populateTags(e),
			Options:                  populateEventOptions(),
			Status:                   mobilizon.EventStatusConfirmed,
		}

		if e.ImageURL != "" {
			vars.ImageURL = e.ImageURL
		}

		var existingUuid = &uuid.UUID{}

		Log.Debug("Checking for existing event", "eventKey", eventKey(e))

		// guard clauses
		if _, ok := existing[eventKey(e)]; ok {

			Log.Debug("Found a cached event", "key", eventKey(e))
			Log.Trace("Found a cached event", "event", spew.Sdump(existing[eventKey(e)].UUID))
			*existingUuid = existing[eventKey(e)].UUID
			created[eventKey(e)] = existing[eventKey(e)]

		} else {
			Log.Debug("Searching for existing events", "title", e.Title, "location", e.Location, "date", e.Date)
			exists, uuid, err := mobClient.EventExists(
				ctx,
				e.Title,
				e.Location,
				e.Date,
			)

			if err != nil {
				Log.Error("Error searching for a matching event", "error", err)
			}

			if exists {
				created[eventKey(e)] = ExistingEvent{*uuid, e}
				existingUuid = uuid
			}
		}

		if *opts.NoOp {
			continue
		}

		// if the source event has changes add the UUID so the client knows
		// to do an update operation instead of a create operation
		if *existingUuid != uuid.Nil {
			if !reflect.DeepEqual(e, existing[eventKey(e)].Event) {
				Log.Debug("Update", "uuid", existingUuid)
				Log.Trace("Update", "saved", spew.Sdump(existing[eventKey(e)].Event), "event", spew.Sdump(e))
				vars.UUID = existingUuid
				if _, err := mobClient.UpdateEvent(ctx, vars); err != nil {
					Log.Error("Error updating event", "error", err)
					// it could be a transient error, cache the cached version
					// again so that we try to update again next time
					created[eventKey(e)] = existing[eventKey(e)]
				} else {
					// cache the updated event
					created[eventKey(e)] = ExistingEvent{*existingUuid, e}
				}
				continue
			} else {
				// the event hasn't changed, there's nothing to do
				continue
			}
		}

		uuid, err := mobClient.CreateEvent(ctx, vars)
		if err == nil {
			created[eventKey(e)] = ExistingEvent{*uuid, e}
			Log.Debug("Created", "uuid", *uuid)
		} else if err.Error() == "returned error 401: {\"data\":null}" {
			mobClient.RefreshToken(ctx, *opts.AuthConfig)
		} else {
			Log.Error("Error creating event", "error", err)
		}
		time.Sleep(30000)
	}
	Log.Debug("Saving existing events list")
	Log.Trace("Saving existing events list", "events", spew.Sdump(created))
	saveExistingEvents()
}

// populateTags constructs an eventTags object for the createEvent mutation
func populateTags(e concertcloud.Event) []*string {
	return []*string{
		&e.Location,
		&e.City,
	}
}

// populateEventOptions creates a default eventOptionsInput object
// FIXME should od this in init()
func populateEventOptions() mobilizon.EventOptionsInput {
	tz := *opts.Timezone
	showStart := true
	showEnd := false
	moderation := mobilizon.EventCommentModeration("ALLOW_ALL")
	return mobilizon.EventOptionsInput{
		CommentModeration: &moderation,
		ShowStartTime:     &showStart,
		ShowEndTime:       &showEnd,
		Timezone:          &tz,
	}
}

// populateCategory takes an event and returns either the event's own
// category if it is found in the list of Mobilizòn's event categories or
// the default category
// FIXME refactor this as an Event object method. Make the default a constant.
func populateCategory(e concertcloud.Event) mobilizon.EventCategory {
	if slices.Contains(mobilizon.AllEventCategory, mobilizon.EventCategory(e.Type)) {
		return mobilizon.EventCategory(e.Type)
	}
	return mobilizon.EventCategory("MUSIC")
}
