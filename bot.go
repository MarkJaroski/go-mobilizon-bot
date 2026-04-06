package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
	"github.com/gen2brain/avif"
	"github.com/google/uuid"

	"github.com/markjaroski/go-mobilizon-bot/concertcloud"
	"github.com/markjaroski/go-mobilizon-bot/mobilizon"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/pflag"

	"golang.org/x/image/draw"
)

const CC_PLUG = "Help promote your favourite venues with: https://concertcloud.live/contribute"
const DEFAULT_IMAGE_URL = "https://mobilisons.ch/img/mobilizon_default_card.png"
const MAX_IMG_SIZE = 1024 * 800 // 800kb
const IMAGE_RESIZE_WIDTH = 600
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

// main still does too much of the work FIXME
func main() {
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
		registration, err = mobilizon.RegisterApp(context.Background(), conf)
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
	if err = mobClient.EnsureAuthorization(context.Background(), *opts.AuthConfig); err != nil {
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
			HTTPClient: mobClient.HTTPClient(context.Background()),
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
		resp, err := ccClient.GetEvents(context.Background(), params)
		// Fetch some concerts from Concert Cloud
		if err != nil {
			Log.Error("error", err)
		}

		events = resp.Data
	}

	createEvents(events)
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

// Convert the concert cloud Address from an event into AddressInput for Mobilizon
func addressToAddressInput(e concertcloud.Event) mobilizon.AddressInput {
	geo := e.Address.Geolocacation.Coordinates
	offset := time.Duration(e.Offset * int(time.Second))
	tzName := "UTC" + fmt.Sprintf("%+.0f", offset.Hours())
	return mobilizon.AddressInput{
		Geom:        fmt.Sprint("%.8f", geo[0], geo[1]),
		Street:      e.Address.HouseNumber + " " + e.Address.Street, // mobilizon uses this order
		Locality:    e.Address.Locality,
		PostalCode:  e.Address.PostCode,
		Region:      e.Address.State,
		Country:     e.Address.Country,
		Description: e.Location,
		Type:        "",
		Url:         e.SourceURL,
		Timezone:    tzName,
	}
}

// createEvents loops through all of the events in the json input, sets up
// their variables map, and runs createEvents on them
func createEvents(events []concertcloud.Event) {

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

		var uuid = &uuid.UUID{}

		Log.Debug("Checking for existing events", "eventKey", eventKey(e))

		// guard clauses
		if _, ok := existing[eventKey(e)]; ok {

			Log.Debug("Found a cached event", "event", spew.Sdump(existing[eventKey(e)].UUID))
			*uuid = existing[eventKey(e)].UUID
			created[eventKey(e)] = existing[eventKey(e)]

			if !reflect.DeepEqual(e, existing[eventKey(e)]) {

				Log.Debug("Update", "saved", spew.Sdump(existing[eventKey(e)]), "event", spew.Sdump(e))
				if *opts.NoOp {
					continue
				}

				created[eventKey(e)] = ExistingEvent{UUID: *uuid, Event: e}

				// FIXME still needs work
				// updateEvent(vars)

			}
			continue
		}

		Log.Debug("Searching for existing events", "title", e.Title, "location", e.Location, "date", e.Date)
		exists, uuid, err := mobClient.EventExists(
			e.Title,
			e.Location,
			e.Date,
		)
		if err != nil {
			Log.Error("Error searching for a matching event", "error", err)
			continue
		}
		if exists {
			created[eventKey(e)] = ExistingEvent{*uuid, e}
			continue
		}

		e.Comment = e.Comment + " <p/><p> " + CC_PLUG

		vars := mobilizon.CreateEventParams{
			Title:                    e.Title,
			Description:              e.Comment,
			BeginsOn:                 e.Date,
			EndsOn:                   e.Date.Add(time.Hour * 2),
			Category:                 populateCategory(e),
			Visibility:               mobilizon.EventVisibility("PUBLIC"),
			JoinOptions:              mobilizon.EventJoinOptions("EXTERNAL"),
			PhysicalAddress:          addressToAddressInput(e),
			OnlineAddress:            e.URL,
			ExternalParticipationURL: e.URL,
			Draft:                    *opts.Draft,
			OrganizerActorId:         actorID,
			AttributedToId:           groupID,
			Tags:                     populateTags(e),
			Options:                  populateEventOptions(),
		}
		uuid, err = mobClient.CreateEvent(context.Background(), vars)
		if err == nil {
			created[eventKey(e)] = ExistingEvent{*uuid, e}
		}
	}
	Log.Debug("Saving existing events list", "events", spew.Sdump(created))
	saveExistingEvents()
}

// FIXME: this is left over from earlier versions
//
// populateVariables takes an Event object from the json input and returns
// a map which can be used as the variables input for the Mobilizòn GraphQL
// mutations createEvent or updateEvent
func populateVariables(e concertcloud.Event) (map[string]interface{}, error) {
	// add a plug for ConcertCloud
	vars := map[string]interface{}{}
	// if we have a UUID fetch the corresponding eventId and use it
	e = populateImageUrl(e)
	path, err := downloadFile(e.ImageURL)
	if err != nil {
		Log.Error("Media download error", "URL", e.ImageURL, "path", path)
		path, _ = downloadFile(DEFAULT_IMAGE_URL)
	}
	uuid, err := mobClient.UploadMediaFile(context.Background(), path)
	if err != nil {
		Log.Error("Media uploade error", "URL", e.ImageURL, "path", path, "uuid", uuid)
		return vars, err
	}
	mi := new(mobilizon.MediaInput)
	mi.MediaUuid = *uuid
	vars["picture"] = mi
	return vars, err
}

// populateImageUrl validates the imageUrl of an event object from the json
// input and if necessary finds one from the event URL. It updates the
// ImageUrl field of the Event object in place.
func populateImageUrl(e concertcloud.Event) concertcloud.Event {
	if e.ImageURL != "" && e.ImageURL != e.SourceURL && !strings.HasSuffix(e.ImageURL, "/") {
		return e
	}

	Log.Info("No image found for", "url", e.URL)
	e.ImageURL = DEFAULT_IMAGE_URL
	return e
}

// populateTags constructs an eventTags object for the createEvent mutation
func populateTags(e concertcloud.Event) []string {
	return []string{
		e.Location,
		e.City,
	}
}

// populateEventOptions creates a default eventOptionsInput object
// FIXME should od this in init()
func populateEventOptions() mobilizon.EventOptionsInput {
	tz := *opts.Timezone
	return mobilizon.EventOptionsInput{
		CommentModeration: mobilizon.EventCommentModeration("ALLOW_ALL"),
		ShowStartTime:     true,
		ShowEndTime:       false,
		Timezone:          tz,
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

// downloadFile downloads a file from a given URL and returns the local
// file path or "" and an error or nil
func downloadFile(URL string) (string, error) {
	// if this is a data URL just return it. The uplaod function will deal.
	if strings.HasPrefix(URL, "data:") {
		return URL, nil
	}

	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return "", errors.New(fmt.Sprintf("Received response code %d for %s", response.StatusCode, URL))
	}

	// get tmp filename
	f, err := os.CreateTemp("", "cc2mob.")
	if err != nil {
		return f.Name(), err
	}

	//Create a empty file
	file, err := os.Create(f.Name())
	if err != nil {
		return f.Name(), err
	}
	defer file.Close()

	//Write the bytes to the file
	if response.ContentLength > MAX_IMG_SIZE || strings.HasSuffix(URL, ".avif") {
		err = thumbnail(response.Body, file, response.Header.Get("Content-Type"), IMAGE_RESIZE_WIDTH)
	} else {
		_, err = io.Copy(file, response.Body)
	}
	if err != nil {
		return f.Name(), err
	}

	return f.Name(), nil
}

// thumbnail creates a resized image from the reader and writes it to
// the writer. The mimetype determines how the image will be decoded
// and must be either "image/jpeg" or "image/png". The desired width
// of the thumbnail is specified in pixels, and the resulting height
// will be calculated to preserve the aspect ratio.
func thumbnail(r io.Reader, w io.Writer, mimetype string, width int) error {
	var src image.Image
	var err error

	switch mimetype {
	case "image/jpeg":
		src, err = jpeg.Decode(r)
	case "image/png":
		src, err = png.Decode(r)
	case "image/avif":
		src, err = avif.Decode(r)
	default:
		err = errors.New("Unknown MIME Type " + mimetype)
	}

	if err != nil {
		return err
	}

	Log.Debug("Resizing image", "MIME Type", mimetype)

	ratio := (float64)(src.Bounds().Max.Y) / (float64)(src.Bounds().Max.X)
	height := int(math.Round(float64(width) * ratio))

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	draw.NearestNeighbor.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)

	err = jpeg.Encode(w, dst, nil)
	if err != nil {
		return err
	}

	return nil
}
