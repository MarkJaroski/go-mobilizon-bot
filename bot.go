// an event importer bot for Mobilizòn, primarily for importing events from
// ConcertCloud.live
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
	"github.com/gen2brain/avif"
	"github.com/gocolly/colly"
	"github.com/hasura/go-graphql-client"
	"github.com/otiai10/opengraph"
	"github.com/vincent-petithory/dataurl"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/spf13/pflag"

	"golang.org/x/image/draw"
)

const (
	ccPlug              = "Help promote your favourite venues with: https://concertcloud.live/contribute"
	defaultImageURL     = "https://mobilisons.ch/img/mobilizon_default_card.png"
	maxImageSize        = 1024 * 800 // 800kb
	imageResizeWidth    = 600
	serverCrashWaitTime = time.Duration(1 * int64(time.Minute))
	addrFile            = "addrs.json"
	existsFile          = "exists.json"
)

// Options represents the full set of command-line options for the bot
type Options struct {
	MobilizonURL *string
	City         *string
	Country      *string
	Limit        *string
	Page         *string
	Radius       *string
	Date         *string
	File         *string
	AuthConfig   *string
	Config       *string
	ActorID      *string
	GroupID      *string
	Timezone     *string
	NoOp         *bool
	Register     *bool
	Authorize    *bool
	Draft        *bool
	Debug        *bool
	AddrsFile    *string
	ExistsFile   *string
}

var opts Options

// Response represents the json reponse from https://api.concertcloud.com/
// and is used to Unmarshal that json
// FIXME it might be possible to import this from the official repo
type Response struct {
	Event    []Event `json:"data"`
	Page     int     `json:"page"`
	Limit    int     `json:"limit"`
	Total    int     `json:"total"`
	LastPage int     `json:"last_page"`
}

// Event represents the Event objects which is the main part of the
// concertcloud response.
// FIXME it might be possible to import this from the official repo
type Event struct {
	Title     string    `json:"title"`
	Location  string    `json:"location"`
	City      string    `json:"city"`
	Country   string    `json:"country"`
	URL       string    `json:"url"`
	Comment   string    `json:"comment"`
	Type      string    `json:"type"`
	SourceURL string    `json:"sourceUrl"`
	Date      time.Time `json:"date"`
	ImageURL  string    `json:"imageUrl"`
	MobUUID   string    `json:"mobilizonUuid"`
}

// UUID represents the GraphQL UUID type
// FIXME move to a library
type UUID string

// MediaUpload represents the GraphQL MediaUpload type
// FIXME move to a library
type MediaUpload struct {
	UUID UUID `json:"uuid"`
}

// MediaData represents the mediaUpload object of a GraphQL mediaUpload mutation
// FIXME move to a library
type MediaData struct {
	Upload MediaUpload `json:"uploadMedia"`
}

// MediaResponse represents the response object of a GraphQL mediaUpload mutation
// FIXME move to a library
type MediaResponse struct {
	Data MediaData `json:"data"`
}

// Address represents the OpenStreetMap address for a given place
type Address struct {
	Amenity       string `json:"amenity"`
	HouseNumber   string `json:"house_number"`
	Road          string `json:"road"`
	Neighbourhood string `json:"neighbourhood"`
	City          string `json:"city"`
	County        string `json:"county"`
	State         string `json:"state"`
	ISOCode       string `json:"ISO3166-2-lvl4"`
	PostCode      string `json:"postcode"`
	Country       string `json:"country"`
	CountryCode   string `json:"country_code"`
}

// Place represents a place as returned by openstreetmap
type Place struct {
	PlaceID     int     `json:"place_id"`
	Name        string  `json:"name"`
	Lat         string  `json:"lat"`
	Lon         string  `json:"lon"`
	Type        string  `json:"type"`
	Address     Address `json:"address"`
	DisplayName string  `json:"display_name"`
}

// NominatumResponse represents the response returned from OpenStreetMap
type NominatumResponse []Place

// Point represents the latitude and longitude of a place in Mobilizòn
// FIXME move to a library
type Point string

// AddressInput represents address data in Mobilizòn GraphQL mutations like
// createEvent and updateEvent
// FIXME move to a library
type AddressInput struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Locality    string `json:"locality"`
	PostalCode  string `json:"postalCode"`
	Street      string `json:"street"`
	Country     string `json:"country"`
	Region      string `json:"region"`
	Geom        Point  `json:"geom"`
}

// MediaInput represents media data in Mobilizòn GraphQL mutations like
// createEvent and updateEvent
type MediaInput struct {
	// FIXME move to a library
	MediaUUID UUID `json:"mediaUuid"`
}

// NominatumBaseURL is the URL we use to call nominatim
var NominatumBaseURL = "https://nominatim.openstreetmap.org/search"

// EventCategory represents the list of possible event categories present
// in Mobilizòn. Obviously this list must be maintained here as the list in
// the Mobilizòn codebase changes.
// FIXME move to a library
type EventCategory string

// Arts ... event Categories
const (
	Arts                        EventCategory = "ARTS"
	AutoBoatAir                 EventCategory = "AUTO_BOAT_AIR"
	BookClubs                   EventCategory = "BOOK_CLUBS"
	Business                    EventCategory = "BUSINESS"
	Causes                      EventCategory = "CAUSES"
	Comedy                      EventCategory = "COMEDY"
	Community                   EventCategory = "COMMUNITY"
	Crafts                      EventCategory = "CRAFTS"
	FamilyEducation             EventCategory = "FAMILY_EDUCATION"
	FashionBeauty               EventCategory = "FASHION_BEAUTY"
	FilmMedia                   EventCategory = "FILM_MEDIA"
	FoodDrink                   EventCategory = "FOOD_DRINK"
	Games                       EventCategory = "GAMES"
	Health                      EventCategory = "HEALTH"
	LanguageCulture             EventCategory = "LANGUAGE_CULTURE"
	Learning                    EventCategory = "LEARNING"
	Lgbtq                       EventCategory = "LGBTQ"
	Meeting                     EventCategory = "MEETING"
	MovementsPolitics           EventCategory = "MOVEMENTS_POLITICS"
	Music                       EventCategory = "MUSIC"
	Networking                  EventCategory = "NETWORKING"
	OutdoorsAdventure           EventCategory = "OUTDOORS_ADVENTURE"
	Party                       EventCategory = "PARTY"
	PerformingVisualArts        EventCategory = "PERFORMING_VISUAL_ARTS"
	Pets                        EventCategory = "PETS"
	Photography                 EventCategory = "PHOTOGRAPHY"
	ScienceTech                 EventCategory = "SCIENCE_TECH"
	SpiritualityReligionBeliefs EventCategory = "SPIRITUALITY_RELIGION_BELIEFS"
	Sports                      EventCategory = "SPORTS"
	Theatre                     EventCategory = "THEATRE"
)

// EventTypeStrings are the strings accepted by the GraphQL interface
var EventTypeStrings = []string{
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
// FIXME move to a library
type EventVisibility string

// Private, etc, are the enumerated EventVisibility types
const (
	Private    EventVisibility = "PRIVATE"
	Public     EventVisibility = "PUBLIC"
	Restricted EventVisibility = "RESTRICTED"
	Unlisted   EventVisibility = "UNLISTED"
)

// EventJoinOptions represents the EventJoinOptions Mobilizòn GraphQL type
// FIXME move to a library
type EventJoinOptions string

// Free: represents unrestricted joining
// External: represents an event managed outside of Mobilizòn
const (
	Free     EventJoinOptions = "FREE"
	External EventJoinOptions = "EXTERNAL"
)

// DateTime represents the DateTime Mobilizòn GraphQL type
// FIXME move to a library
type DateTime string

// EventCommentModeration represents the EventCommentModeration Mobilizòn
// GraphQL type
// FIXME move to a library
type EventCommentModeration string

// AllowAll: anyone may register for the event
// Closed: no one may Register
// Moderated: anyone may register, subject to approval
const (
	AllowAll  EventCommentModeration = "ALLOW_ALL"
	Closed    EventCommentModeration = "CLOSED"
	Moderated EventCommentModeration = "MODERATED"
)

// Timezone represents the cooresponding Mobilizòn GraphQL type
// FIXME move to a library
type Timezone string

// EventOptionsInput represents the cooresponding Mobilizòn GraphQL type
// FIXME move to a library
type EventOptionsInput struct {
	CommentModeration EventCommentModeration `json:"commentModeration"`
	ShowStartTime     graphql.Boolean        `json:"showStartTime"`
	ShowEndTime       graphql.Boolean        `json:"showEndTime"`
	Timezone          Timezone               `json:"timezone"`
}

// AuthConfig is the OAuth2 response presented by Mobilizòn for
// authorization and reauthorization. Becomes the structure of the auth
// FIXME move to a library
type AuthConfig struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Scopes                string `json:"scopes"`
	TokenType             string `json:"token_type"`
}

// local fields
var auth AuthConfig
var actorID string
var groupID string
var timezone *string
var addrs map[string]AddressInput
var existing map[string]Event
var created map[string]Event
var httpClient *http.Client
var gqlClient *graphql.Client
var addrsFile string
var existsFile string
var authFile string

// Log is our hclog local instance
var Log hclog.Logger

// init sets up logging and initializes the addr map
func init() {
	Log = hclog.New(&hclog.LoggerOptions{
		Name:  "Mobilizon bot",
		Level: hclog.LevelFromString("INFO"),
	})
	addrs = make(map[string]AddressInput)
	existing = make(map[string]Event)
	created = make(map[string]Event)
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

	opts.MobilizonURL = pflag.String("mobilizonurl", "https://mobilisons.ch", "Your Mobilizon base URL")
	opts.City = pflag.String("city", "X", "The concertcloud API param 'city'") // defaults to X to avoid accidental flooding
	opts.Country = pflag.String("country", "", "The concertcloud API param 'country'")
	opts.Limit = pflag.String("limit", "", "The concertcloud API param 'limit'")
	opts.Page = pflag.String("page", "", "The concertcloud API param 'page'")
	opts.Radius = pflag.String("radius", "", "The concertcloud API param 'radius'")
	opts.Date = pflag.String("date", "", "The concertcloud API param 'date'")
	opts.File = pflag.String("file", "", "Instead of fetching from concertcloud, use local file.")
	opts.ActorID = pflag.String("actor", "", "The Mobilizon actor ID to use as the event organizer.")
	opts.GroupID = pflag.String("group", "", "The Mobilizon group ID to use for the event attribution.")
	opts.Timezone = pflag.String("timezone", "Europe/Zurich", "The timezone to use for the event attribution.")
	opts.AuthConfig = pflag.String("authconfig", confdir+"/mobilizon/auth.json", "Use this file for authorization tokens.")
	opts.Config = pflag.String("config", confdir+"/mobilizon", "Use this directory for configuration.")
	opts.NoOp = pflag.Bool("noop", false, "Gather all required information and report on it, but do not create events in Mobilizòn.")
	opts.Register = pflag.Bool("register", false, "Register this bot and quit. A client id will be output.")
	opts.Authorize = pflag.Bool("authorize", false, "Authorize this bot and quit. An auth token and renew token will be output.")
	opts.Draft = pflag.Bool("draft", false, "Create events in draft mode.")
	opts.Debug = pflag.Bool("debug", false, "Debug mode.")

	pflag.Parse()

	if *opts.Config != confdir+"/mobilizon" {
		*opts.AuthConfig = *opts.Config + "/auth.json"
	}

	if *opts.Register {
		registerApp()
		return
	}

	// do the authorization regardless ...
	authorizeApp()
	// and if that's all there is to do exit
	if *opts.Authorize {
		return
	}

	// set up the ContentCloud query
	ccQuery := ""
	if *opts.City != "X" && *opts.City != "" {
		ccQuery = fmt.Sprintf("%s&city=%s", ccQuery, url.QueryEscape(*opts.City))
	}
	if *opts.Country != "" {
		ccQuery = fmt.Sprintf("%s&country=%s", ccQuery, url.QueryEscape(*opts.Country))
	}
	if *opts.Limit != "" {
		ccQuery = fmt.Sprintf("%s&limit=%s", ccQuery, *opts.Limit)
	}
	if *opts.Page != "" {
		ccQuery = fmt.Sprintf("%s&page=%s", ccQuery, *opts.Page)
	}
	if *opts.Radius != "" {
		ccQuery = fmt.Sprintf("%s&radius=%s", ccQuery, *opts.Radius)
	}
	if *opts.Date != "" {
		ccQuery = fmt.Sprintf("%s&date=%s", ccQuery, url.QueryEscape(*opts.Date))
	}
	if *opts.Debug {
		Log.SetLevel(hclog.LevelFromString("DEBUG"))
	}

	actorID = *opts.ActorID
	groupID = *opts.GroupID

	addrsFile = *opts.Config + "/" + addrFile
	existsFile = *opts.Config + "/" + existsFile

	// set up an HTTPClient with automated retries
	retryClient := retryablehttp.NewClient()
	retryClient.RetryWaitMin = serverCrashWaitTime
	retryClient.RetryWaitMax = time.Duration(10 * int64(time.Minute))
	retryClient.RetryMax = 120
	retryClient.CheckRetry = mobilizònRetryPolicy
	retryClient.Backoff = mobilizònErrorBackoff

	retryClient.Logger = Log

	httpClient = retryClient.StandardClient()

	gqlClient = graphql.NewClient(*opts.MobilizonURL+"/api", httpClient)
	gqlClient = gqlClient.WithRequestModifier(func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	})

	// this will hold our json object whether local or from ConcertCloud
	var events []Event

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
		// Fetch some concerts from Concert Cloud
		fetchURL := fmt.Sprintf("%s?%s", "https://api.concertcloud.live/api/events", ccQuery)
		response, err := http.Get(fetchURL)
		if err != nil {
			Log.Error("error", err)
			os.Exit(1) // no point in continuing
		}

		responseData, err := io.ReadAll(response.Body)
		if err != nil {
			Log.Error("", err)
			os.Exit(1) // no point in continuing
		}

		var jsonEventInput Response
		json.Unmarshal(responseData, &jsonEventInput)
		events = jsonEventInput.Event
	}

	fetchAddrs(events)
	createEvents(events)
}

// fetchAddrs loads the local addr.json file cache and then attempts to
// fetch any missing addresses from OpenStreetMap and Mobilizòn
func fetchAddrs(events []Event) {
	// Read the local file, if it exists. We can trap errors here
	// since we can just recreate the file if necessary.
	dat, err := os.ReadFile(addrsFile)
	if err != nil {
		Log.Error(err.Error())
	}
	err = json.Unmarshal(dat, &addrs)
	if err != nil {
		Log.Error(err.Error())
	}

	for i := 0; i < len(events); i++ {
		fetchAddr(events[i])
	}

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
	}
	err = json.Unmarshal(dat, &existing)
	if err != nil {
		Log.Error(err.Error())
	}
}

func saveExistingEvents() {
	data, err := json.MarshalIndent(&created, "", " ")
	if err != nil {
		Log.Error(err.Error())
	}
	err = os.WriteFile(existsFile, data, 0600)
	if err != nil {
		Log.Error(err.Error())
	}
}

// fetchAddr uses OpenStreetMap Nominatim to create a query string which
// should in almost all cases return the correct location object when run
// against the Mobilizòn address search.
func fetchAddr(event Event) {
	Log.Debug("Searching for: ", "location", event.Location)

	// if we already have the don't bother with the query
	_, ok := addrs[event.Location]
	if ok {
		Log.Debug("Skipping cached location", "location", event.Location)
		return
	}

	// get the addr from OpenStreetMap first
	query := fetchOSMAddr(event)
	Log.Debug("Returned from OSM:", "query", query)

	// now query Mobilizòn to make sure we use the same address object
	var s struct {
		SearchAddress []AddressInput `graphql:"searchAddress(query: $query)"`
	}
	vars := map[string]interface{}{
		"query": query,
	}
	err := gqlClient.Query(context.Background(), &s, vars)
	if err != nil {
		Log.Error("fetchAddrs", err)
		time.Sleep(3 * time.Second)
		gqlClient.Query(context.Background(), &s, vars)
	}

	if len(s.SearchAddress) == 0 {
		Log.Info("Address not found: ", "query", query)
		return
	}

	for _, a := range s.SearchAddress {
		Log.Debug("Mobilizòn returned: '" + a.Description + " " + a.Street + " " + a.Locality + " for " + event.Location + " " + event.City)
		if a.Description == event.Location && a.Locality == event.City {
			addrs[event.Location] = a
			return
		}
	}

	// just use the last one
	addrs[event.Location] = s.SearchAddress[len(s.SearchAddress)-1]
}

// fetchOSMAddr takes a single Event object from the json input and returns
// a query string for Mobilizòn which should return the location object
// which Mobilizòn has constructed for the event address.
//
// Doing it this way improves our chances of getting an exact hit when we
// run the query against Mobilizòn itself.
func fetchOSMAddr(event Event) string {

	var addr Place

	Log.Debug("Doing lookup in OpenStreetMap")
	var querystring = fmt.Sprintf("amenity=%s&city=%s&format=json&addressdetails=1",
		url.QueryEscape(event.Location),
		url.QueryEscape(event.City))
	var nurl = fmt.Sprintf("%s?%s", NominatumBaseURL, querystring)
	nresp, err := http.Get(nurl)

	if err != nil {
		Log.Debug(err.Error())
		os.Exit(1)
	}

	addrData, err := io.ReadAll(nresp.Body)
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}
	var addrObject NominatumResponse
	json.Unmarshal(addrData, &addrObject)

	if len(addrObject) == 0 {
		Log.Debug("OSM Place Not found:", "location", event.Location, "city", event.City)
		return event.Location + " " + event.City
	} else if len(addrObject) == 1 {
		addr = addrObject[0]
	} else {
		for _, p := range addrObject {
			if p.Type == "nightclub" || p.Type == "bar" || p.Type == "restaurant" || p.Type == "theatre" || p.Type == "cinema" || p.Type == "arts_centre" {
				Log.Debug("Addr Type:", p.Type)
				addr = p
				break
			}
		}
	}

	return event.Location + " " + addr.Address.Road + " " + addr.Address.City
}

// disambiguates the URI for a given event, just in case the venue does not
// differentiate.
func getEventKey(e Event) string {
	var url = e.URL
	match, _ := regexp.MatchString("#", e.URL)
	if match {
		url = url + ":"
	} else {
		url = url + "#"
	}
	url = url + e.Date.Format(time.RFC3339)
	return url
}

// createEvents loops through all of the events in the json input, sets up
// their variables map, and runs createEvents on them
func createEvents(events []Event) {
	loadExistingEvents()
	for i := 0; i < len(events); i++ {
		event := events[i]
		// Do not upload events from bejazz.ch. They don't like us.
		// opt out FIXME this should be loaded from a file or something
		match, _ := regexp.MatchString("bejazz.ch", event.URL)
		if match {
			Log.Info("Skipping BeJazz.")
			continue
		}
		// trim the title to produce better matches
		event.Title = strings.TrimSpace(event.Title)
		// titles must be at least 3 characters long in Mobilizòn
		if utf8.RuneCountInString(event.Title) < 3 {
			event.Title = event.Title + " ..."
		}
		Log.Debug("Checking for existing events", "eventKey", getEventKey(event))
		// guard clauses
		if _, ok := existing[getEventKey(event)]; ok {
			Log.Debug("Found a cached event")
			event.MobUUID = existing[getEventKey(event)].MobUUID // events never come in with a MobUUID
			created[getEventKey(event)] = existing[getEventKey(event)]
			if !reflect.DeepEqual(event, existing[getEventKey(event)]) {
				Log.Debug("Update", "saved", spew.Sdump(existing[getEventKey(event)]), "event", spew.Sdump(event))
				if *opts.NoOp {
					continue
				}
				created[getEventKey(event)] = event
				vars, err := populateVariables(event)
				if err != nil {
					Log.Error("Error populating vars", "error", err, "vars", spew.Sdump(vars))
					continue
				}
				// FIXME still needs work
				// updateEvent(vars)
			}
			continue
		}
		if ok, uuid := eventExists(event); ok {
			event.MobUUID = uuid
			created[getEventKey(event)] = event
			continue
		}
		if *opts.NoOp {
			continue
		}
		vars, err := populateVariables(event)
		if err != nil {
			Log.Error("Error populating vars", "error", err, "vars", spew.Sdump(vars))
			continue
		}
		uuid, err := createEvent(vars)
		if err == nil {
			event.MobUUID = uuid
			created[getEventKey(event)] = event
		}
	}
	saveExistingEvents()
}

// populateVariables takes an Event object from the json input and returns
// a map which can be used as the variables input for the Mobilizòn GraphQL
// mutations createEvent or updateEvent
func populateVariables(e Event) (map[string]interface{}, error) {
	// add a plug for ConcertCloud
	e.Comment = e.Comment + " <p/><p> " + ccPlug
	vars := map[string]interface{}{
		"organizerActorId":         graphql.ID(actorID),
		"attributedToId":           graphql.ID(groupID),
		"category":                 populateCategory(e),
		"visibility":               EventVisibility("PUBLIC"),
		"joinOptions":              EventJoinOptions("EXTERNAL"),
		"title":                    e.Title,
		"description":              e.Comment,
		"physicalAddress":          addrs[e.Location],
		"beginsOn":                 DateTime(e.Date.Format(time.RFC3339)),
		"endsOn":                   DateTime(e.Date.Add(time.Hour * 2).Format(time.RFC3339)),
		"draft":                    graphql.Boolean(*opts.Draft),
		"onlineAddress":            e.URL,
		"externalParticipationUrl": e.URL,
		"tags":                     populateTags(e),
		"options":                  populateEventOptions(),
	}
	// if we have a UUID fetch the corresponding eventId and use it
	e = populateImageURL(e)
	if e.MobUUID != "" {
		eventID, err := fetchEvent(e.MobUUID)
		if err != nil {
			return vars, err
		}
		vars["id"] = eventID
		// skip the image upload for updates
		// return vars, err
	}
	path, err := downloadFile(e.ImageURL)
	if err != nil {
		Log.Error("Media download error", "URL", e.ImageURL, "path", path)
		path, _ = downloadFile(defaultImageURL)
	}
	uuid, err := uploadEventImage(path)
	if err != nil {
		Log.Error("Media uploade error", "URL", e.ImageURL, "path", path, "uuid", uuid)
		return vars, err
	}
	mi := new(MediaInput)
	mi.MediaUUID = uuid
	vars["picture"] = mi
	return vars, err
}

// populateImageURL validates the imageUrl of an event object from the json
// input and if necessary finds one from the event URL. It updates the
// ImageUrl field of the Event object in place.
func populateImageURL(e Event) Event {
	if e.ImageURL != "" && e.ImageURL != e.SourceURL && !strings.HasSuffix(e.ImageURL, "/") {
		return e
	}
	// fetch the opengraph image for the event if there is no event image
	e.ImageURL = fetchOGImageURL(e.URL)
	if strings.HasPrefix(e.ImageURL, "http") {
		return e
	}
	// fetch a backup image if we don't already have something
	e.ImageURL = guessEventImage(e.URL)
	if strings.HasPrefix(e.ImageURL, "http") {
		return e
	}
	Log.Info("No image found for", "url", e.URL)
	e.ImageURL = defaultImageURL
	return e
}

// uploadEventImage uploads the file at the given path, and returns its
// mobilison IT and any error which occurs in the process
func uploadEventImage(path string) (UUID, error) {
	multi, err := newfileUploadRequest(path)
	if err != nil {
		Log.Error("Error constructing media request", "path", path, "error", err)
		return "", err
	}

	response, err := httpClient.Do(multi)
	if err != nil {
		Log.Error("Error uploading image", "path", path, "error", err)
		return "", err
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		Log.Error("Error getting media response", "path", path, "error", err)
		return "", err
	}

	var mediaObject MediaResponse
	json.Unmarshal(responseData, &mediaObject)
	if mediaObject.Data.Upload.UUID == "" {
		err = errors.New("Image id not found in upload response. " + path)
	}
	return (UUID)(mediaObject.Data.Upload.UUID), err
}

// populateTags constructs an eventTags object for the createEvent mutation
func populateTags(e Event) []string {
	return []string{
		e.Location,
		e.City,
	}
}

// populateEventOptions creates a default eventOptionsInput object
// FIXME should od this in init()
func populateEventOptions() EventOptionsInput {
	tz := Timezone(*opts.Timezone)
	return EventOptionsInput{
		CommentModeration: EventCommentModeration("ALLOW_ALL"),
		ShowStartTime:     graphql.Boolean(true),
		ShowEndTime:       graphql.Boolean(false),
		Timezone:          tz,
	}
}

// populateCategory takes an event and returns either the event's own
// category if it is found in the list of Mobilizòn's event categories or
// the default category
// FIXME refactor this as an Event object method. Make the default a constant.
func populateCategory(e Event) EventCategory {
	if slices.Contains(EventTypeStrings, e.Type) {
		return EventCategory(e.Type)
	}
	return EventCategory("MUSIC")
}

// createEvent implements the Mobilizòn graphQL createEvent mutation
// taking a map of strings to objects to populate its variables
// FIXME split this out to a library
func createEvent(vars map[string]interface{}) (string, error) {
	var m struct {
		CreateEvent struct {
			ID   string
			UUID string
		} `graphql:"createEvent(organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, category: $category, visibility: $visibility, description: $description, physicalAddress: $physicalAddress, beginsOn: $beginsOn, endsOn: $endsOn, draft: $draft, onlineAddress: $onlineAddress, externalParticipationUrl: $externalParticipationUrl, tags: $tags, joinOptions: $joinOptions, options: $options, picture: $picture)"`
	}
	err := gqlClient.Mutate(context.Background(), &m, vars)
	if err != nil {
		Log.Error("Error creating event", "error", err, "vars", spew.Sdump(vars))
		return "", err
	}
	Log.Info("Created Event", "id", m.CreateEvent.ID, "UUID", m.CreateEvent.UUID)
	return m.CreateEvent.UUID, err
}

// updateEvent implements the Mobilizòn GraphQL updateEvent mutation
// FIXME split this out to a library
func updateEvent(vars map[string]interface{}) (string, error) {
	var m struct {
		UpdateEvent struct {
			ID   string
			UUID string
		} `graphql:"updateEvent(eventId: $id, organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, category: $category, visibility: $visibility, description: $description, physicalAddress: $physicalAddress, beginsOn: $beginsOn, endsOn: $endsOn, draft: $draft, onlineAddress: $onlineAddress, externalParticipationUrl: $externalParticipationUrl, tags: $tags, joinOptions: $joinOptions, options: $options, picture: $picture)"`
	}
	err := gqlClient.Mutate(context.Background(), &m, vars)
	if err != nil {
		Log.Error("Error updating event", "error", err, "vars", spew.Sdump(vars))
		return "", err
	}
	Log.Info("Updated Event", "id", m.UpdateEvent.ID, "UUID", m.UpdateEvent.UUID)
	return m.UpdateEvent.ID, err
}

// FIXME split this out to a library
func fetchEvent(uuid string) (graphql.ID, error) {
	Log.Debug("Attempting to fetch event by uuid", "uuid", uuid)
	var q struct {
		Event struct {
			ID graphql.ID `json:"id"`
		} `graphql:"event(uuid: $uuid)"`
	}
	type UUID string
	vars := map[string]interface{}{
		"uuid": UUID(uuid),
	}
	err := gqlClient.Query(context.Background(), &q, vars)
	if err != nil {
		Log.Error("Error fetching event", "error", err, "vars", spew.Sdump(vars))
		return "", err
	}
	Log.Debug("Got ID", "id", q.Event.ID)
	return q.Event.ID, nil
}

// registerApp registers an OAuth2 client called "Concert Cloud Bot" and
// and exports the resulting environmental variables as well as printing
// them on the commend line
func registerApp() {

	type Registration struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}

	var posturl = *opts.MobilizonURL + "/apps"
	body := []byte(`name=Concert%20Cloud%20Bot&redirect_uri=https://login.microsoftonline.com/common/oauth2/nativeclient&website=https://concertcloud.live&scope=write:event:create%20write:event:update%20write:media:upload`)
	r, err := http.NewRequest("POST", posturl, bytes.NewBuffer(body))
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}

	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	c := &http.Client{}
	res, err := c.Do(r)
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}

	resData, err := io.ReadAll(res.Body)
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}

	var reg Registration
	json.Unmarshal(resData, &reg)

	os.Setenv("GRAPHQL_CLIENT_ID", reg.ClientID)
	// os.Setenv("GRAPHQL_CLIENT_SECRET", reg.ClientSecret)

	fmt.Println("export GRAPHQL_CLIENT_ID=" + reg.ClientID)
	// fmt.Println("export GRAPHQL_CLIENT_SECRET=" + reg.ClientSecret)
}

// authorizeApp does the OAuth2 authorization handshake using the device
// flow, which seems to work best for Mobilizòn, and nicely avoids the
// problem of having to copy URLs back and forth
func authorizeApp() {
	// Let's first check for a valid refreshToken in our config
	// If that doesn't work then we need to authorize interactively
	err := refreshAuthorization()
	if err == nil {
		return
	}

	Log.Debug("Performing OAuth2 handshake.")

	var posturl = *opts.MobilizonURL + "/login/device/code"
	clientID := os.Getenv("GRAPHQL_CLIENT_ID")

	body := []byte("client_id=" + clientID + "&scope=write:event:create%20write:event:update%20write:media:upload")
	r, err := http.NewRequest("POST", posturl, bytes.NewBuffer(body))
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}

	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	c := &http.Client{}
	res, err := c.Do(r)
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}

	resData, err := io.ReadAll(res.Body)
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}

	type DeviceCodeGrant struct {
		DeviceCode      string `json:"device_code"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		Error           string `json:"error"`
	}

	var resp DeviceCodeGrant
	err = json.Unmarshal(resData, &resp)
	if err != nil {
		Log.Error("Error unmarshaling json:", err.Error())
		os.Exit(1)
	}
	if resp.Error != "" {
		Log.Error("Error getting verification URI", resp.Error)
		os.Exit(1)
	}

	fmt.Println("Please visit this URL and enter the code below " + resp.VerificationURI)
	fmt.Println()
	fmt.Println(resp.UserCode)
	fmt.Println()
	fmt.Println("Then press any key to continue.")

	// wait for input
	fmt.Scanln()

	var tokenURL = *opts.MobilizonURL + "/oauth/token"
	tokenBody := []byte("client_id=" + clientID + "&device_code=" + resp.DeviceCode + "&grant_type=urn:ietf:params:oauth:grant-type:device_code")
	tokreq, err := http.NewRequest("POST", tokenURL, bytes.NewBuffer(tokenBody))
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}

	tokreq.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	tokres, err := c.Do(tokreq)

	resData, err = io.ReadAll(tokres.Body)
	if err != nil {
		Log.Error("", err)
		os.Exit(1)
	}

	err = os.WriteFile(*opts.AuthConfig, resData, 0600)

}

// fetchOGImageURL finds takes the URL of a specific event and returns the
// Open Ggraph image URL found there, if one exists.
// See https://ogp.me/
func fetchOGImageURL(url string) string {
	Log.Debug("Fetching opengraph image url.")

	// get the ogp object
	ogp, err := opengraph.Fetch(url)
	if err != nil {
		Log.Error("fetchOGImage", "error", err)
	}

	if len(ogp.Image) == 0 {
		Log.Debug("No opengraph image found")
		return ""
	}

	// convert URLs to absolute
	ogp.ToAbsURL()

	if strings.Contains(url, ogp.Image[0].URL) {
		Log.Debug("Opengraph image URL is a substring of the base URL")
		return ""
	}

	//but check that it works first
	res, err := http.Head(ogp.Image[0].URL)
	if err != nil {
		Log.Error("fetchOGImage", "error", err)
		return ""
	}

	if res.StatusCode != 200 {
		Log.Error("fetchOGImage", "status", res.StatusCode)
		return ""
	}

	Log.Debug("Returning first opengraph image URL", "url", ogp.Image[0].URL)
	return ogp.Image[0].URL
}

// guessEventImage tries to find a reasonable image on the page found at an
// event URL. If it succeeds it returns the image URL otherwise it returns
// ""
//
// The best image is so far defined as the largest image in a src attribute
// on the page. This is far from ideal, but it's a fallback.
func guessEventImage(url string) string {
	Log.Debug("Attempting to guess an image URL for", "url", url)
	var srcs []string

	c := colly.NewCollector()

	// claim to be a browser
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36"

	c.OnHTML("img[src]", func(e *colly.HTMLElement) {
		i := e.Request.AbsoluteURL(e.Attr("src"))
		srcs = append(srcs, i)
	})
	c.Visit(url)
	c.Wait()

	if len(srcs) < 1 {
		return ""
	}

	// Is biggest best? Well maybe not, but that's what we have to work with.
	var size int64
	var best = -1
	for i, src := range srcs {
		// occassionally we get an inline image
		if strings.HasPrefix(src, "data:") {
			continue
		}
		// sometimes we don't get the absolute URL
		if strings.HasPrefix(src, "/") {
			continue
		}
		if strings.HasSuffix(src, ".svg") {
			continue
		}
		res, err := http.Head(src)
		if err != nil {
			Log.Error("Could not perform HEAD method for image", "src", src, "error", err)
		}
		cl := res.ContentLength
		if cl > size && cl < maxImageSize {
			best = i
			size = cl
		}
		Log.Debug("Choosing image by size", "i", i, "size", size, "cl", cl, "best", best)
	}
	if best == -1 {
		return ""
	}
	return srcs[best]
}

// newfileUploadRequest constructs an http request object for Mobilizòn
// file uploads when given a local file path. It returns the request object
// and an error object.
func newfileUploadRequest(path string) (*http.Request, error) {

	var fileContents []byte
	var fi fs.FileInfo
	if strings.HasPrefix(path, "data:") {
		Log.Debug("newFileUploadRequest", path)
		dataURL, err := dataurl.DecodeString(path)
		if err != nil {
			return nil, err
		}
		fileContents, err = base64.StdEncoding.DecodeString(dataURL.String())
		if err != nil {
			return nil, err
		}
	} else {

		// grab the file
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		// get the contents
		fileContents, err = io.ReadAll(file)
		if err != nil {
			return nil, err
		}

		// get the filename etc
		fi, err = file.Stat()
		if err != nil {
			return nil, err
		}
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
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	r, err := http.NewRequest("POST", *opts.MobilizonURL+"/api", body)
	r.Header.Add("Content-Type", writer.FormDataContentType())
	r.Header.Add("Authorization", "Bearer "+auth.AccessToken)

	return r, err
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
		return "", fmt.Errorf("Received response code %d for %s", response.StatusCode, URL)
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
	if response.ContentLength > maxImageSize || strings.HasSuffix(URL, ".avif") {
		err = thumbnail(response.Body, file, response.Header.Get("Content-Type"), imageResizeWidth)
	} else {
		_, err = io.Copy(file, response.Body)
	}
	if err != nil {
		return f.Name(), err
	}

	return f.Name(), nil
}

// eventExists searches Mobilizòn by event title and date, and then checks
// for a matching event URL. This is usually enough to prevent duplicates,
// however it doesn't work for those venues which do not have unique URLs
// per event.
func eventExists(e Event) (bool, string) {
	Log.Debug("Searching for existing events", "title", e.Title, "date", e.Date.Format(time.RFC3339))
	var s struct {
		SearchEvents struct {
			Total    int `json:"total"`
			Elements []struct {
				ID       graphql.ID `json:"id"`
				UUID     string     `json:"uuid"`
				Title    string     `json:"title"`
				BeginsOn string     `json:"beginsOn"`
			}
		} `graphql:"searchEvents(term: $term, beginsOn: $beginsOn)"`
	}
	vars := map[string]interface{}{
		"term":     e.Title,
		"beginsOn": DateTime(e.Date.Format(time.RFC3339)),
	}
	err := gqlClient.Query(context.Background(), &s, vars)
	if err != nil {
		Log.Error("Error checking if event exists", "error", err)
		if strings.Contains(err.Error(), "token_expired") {
			authorizeApp()
		}
		if strings.Contains(err.Error(), "401") {
			authorizeApp()
		}
	}

	// loop through the events and return true if we have a real match
	for _, el := range s.SearchEvents.Elements {
		// fetch the onlineAddress
		var f struct {
			Event struct {
				OnlineAddress string `json:"onlineAddress"`
			} `graphql:"event(uuid: $uuid)"`
		}
		fvars := map[string]interface{}{
			"uuid": UUID(el.UUID),
		}
		err := gqlClient.Query(context.Background(), &f, fvars)
		if err != nil {
			Log.Debug("Failed fetching event by uuid:", el.UUID, err)
		}

		Log.Debug("Checking URL for a match", "url", e.URL)
		if e.URL == f.Event.OnlineAddress {
			Log.Debug("Found event matching", "url", e.URL)
			return true, el.UUID
		} else if e.URL+"/" == f.Event.OnlineAddress {
			Log.Debug("Found event matching", "url", e.URL, "issue", "no trailing slash")
			return true, el.UUID
		} else if e.URL == f.Event.OnlineAddress+"/" {
			Log.Debug("Found event matching", "url", e.URL, "issue", "trailing slash")
			return true, el.UUID
		}
	}

	Log.Info("Event not found", "title", e.Title, "date", e.Date.Format(time.RFC3339), "location", e.Location)
	return false, ""
}

// refreshAuthorization attempts to use the refresh token from the stored
// auth.json file to obtain a new authorization token
func refreshAuthorization() error {
	// Note that the graphql RefreshToken mutation replies with a very
	// differrent kind of object than the authorization does
	var m struct {
		RefreshToken struct {
			AccessToken  string
			RefreshToken string
		} `graphql:"refreshToken(refreshToken: $rt)"`
	}

	// Read the local file, if it exists. We can trap errors here
	// since we can just recreate the file if necessary.
	dat, err := os.ReadFile(*opts.AuthConfig)
	if err != nil {
		if strings.HasSuffix(err.Error(), "no such file or directory") {
			return err
		}
		Log.Error("Error reading auth file:", err.Error())
	}
	err = json.Unmarshal(dat, &auth)
	if err != nil {
		Log.Error("Error unmarshaling json:", err.Error())
	}

	Log.Debug("Using refresh token: " + auth.RefreshToken)
	variables := map[string]interface{}{
		"rt": auth.RefreshToken,
	}

	// run the refresh token query. We need to resturn any errors from here
	// down because they mean that the refresh has failed and so we'll need
	// to do the regular authorization
	c := graphql.NewClient(*opts.MobilizonURL+"/api", nil)
	err = c.Mutate(context.Background(), &m, variables)
	if err != nil {
		Log.Error("Failed auth token renewal")
		return err
	}
	auth.AccessToken = m.RefreshToken.AccessToken
	auth.RefreshToken = m.RefreshToken.RefreshToken

	data, err := json.MarshalIndent(auth, "", " ")
	if err != nil {
		return err
	}
	err = os.WriteFile(*opts.AuthConfig, data, 0600)
	return err
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

// mobilizònRetryPolicy impements the RetryPolicy interface from
// hashicorp.retryablehttp, which captures the main failure modes cause by
// an ephemeral crash of the Mobilizòn server process
func mobilizònRetryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if resp.Status == "401" {
		refreshAuthorization()
		return true, nil
	}
	if resp.Status < "400" {
		return false, nil
	}
	Log.Debug("Retry Policy Event", "", ctx.Value, "http_status", resp.Status, "error", err)
	if resp.Status >= "405" {
		return true, nil
	}
	return false, nil
}

// mobilizònErrorBackoff implements the Backoff interface from
// hashicorp.retryablehttp, waiting long enough for Mobilizòn to recover
// from an activity-pub related crash
func mobilizònErrorBackoff(minT, maxT time.Duration, attemptNum int, resp *http.Response) time.Duration {
	Log.Error("HTTP Error Backoff Called", "min", minT, "max", maxT, "attempt", attemptNum, "status", resp.Status)
	return serverCrashWaitTime
}
