package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"github.com/hasura/go-graphql-client"
	"github.com/otiai10/opengraph"
	"github.com/rxwycdh/rxhash"
	"github.com/vincent-petithory/dataurl"
	"golang.org/x/oauth2"

	"github.com/spf13/pflag"
)

const CC_PLUG = "Help promote your favourite venues with: https://concertcloud.live/contribute"
const MAX_IMG_SIZE = 1000000

type Options struct {
	City       *string
	Country    *string
	Limit      *string
	Page       *string
	Radius     *string
	Date       *string
	File       *string
	AuthConfig *string
	Config     *string
	ActorID    *string
	GroupID    *string
	Timezone   *string
	NoOp       *bool
	Register   *bool
	Authorize  *bool
	Draft      *bool
}

var opts Options

type Response struct {
	Event    []Event `json:"data"`
	Page     int     `json:"page"`
	Limit    int     `json:"limit"`
	Total    int     `json:"total"`
	LastPage int     `json:"last_page"`
}

type Event struct {
	Title     string    `json:"title"`
	Location  string    `json:"location"`
	City      string    `json:"city"`
	Country   string    `json:"country"`
	URL       string    `json:"url"`
	Comment   string    `json:"comment"`
	Type      string    `json:"type"`
	SourceUrl string    `json:"sourceUrl"`
	ImageUrl  string    `json:"imageUrl"`
	Date      time.Time `json:"date"`
}

type UUID string

type MediaUpload struct {
	Id string `json:"id"`
}

type MediaData struct {
	Upload MediaUpload `json:"uploadMedia"`
}

type MediaResponse struct {
	Data MediaData `json:"data"`
}

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

type Place struct {
	PlaceId     int     `json:"place_id"`
	Name        string  `json:"name"`
	Lat         string  `json:"lat"`
	Lon         string  `json:"lon"`
	Type        string  `json:"type"`
	Address     Address `json:"address"`
	DisplayName string  `json:"display_name"`
}

type NominatumResponse []Place

type Point string

type AddressInput struct {
	Id          int    `json:"id"`
	Description string `json:"description"`
	Locality    string `json:"locality"`
	PostalCode  string `json:"postalCode"`
	Street      string `json:"street"`
	Country     string `json:"country"`
	Region      string `json:"region"`
}

type MediaInput struct {
	MediaId graphql.ID `json:"mediaId"`
}

var NominatumBaseURL = "https://nominatim.openstreetmap.org/search"

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

type EventVisibility string

const (
	PRIVATE    EventVisibility = "PRIVATE"
	PUBLIC     EventVisibility = "PUBLIC"
	RESTRICTED EventVisibility = "RESTRICTED"
	UNLISTED   EventVisibility = "UNLISTED"
)

type EventJoinOptions string

const (
	FREE     EventJoinOptions = "FREE"
	EXTERNAL EventJoinOptions = "EXTERNAL"
)

type DateTime string

type EventCommentModeration string

const (
	ALLOW_ALL EventCommentModeration = "ALLOW_ALL"
	CLOSED    EventCommentModeration = "CLOSED"
	MODERATED EventCommentModeration = "MODERATED"
)

type Timezone string

type EventOptionsInput struct {
	CommentModeration EventCommentModeration `json:"commentModeration"`
	ShowStartTime     graphql.Boolean        `json:"showStartTime"`
	ShowEndTime       graphql.Boolean        `json:"showEndTime"`
	Timezone          Timezone               `json:"timezone"`
}

// For authorization and reauthorization. Becomes the structure of the auth
// config file
type AuthConfig struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             string `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn string `json:"refresh_token_expires_in"`
	Scopes                string `json:"scopes"`
	TokenType             string `json:"token_type"`
}

// make this a global var so we don't have to read the file more than once
var auth AuthConfig

var actorID *string
var groupID *string
var timezone *string

var HttpClient *http.Client
var Client *graphql.Client

func main() {
	// set up our config dir if it's not already there
	confdir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	err = os.Mkdir(confdir+"/mobilizon", 0700)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		log.Fatal(err)
	}

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
	opts.Config = pflag.String("config", confdir+"/mobilizon/config.json", "Use this file for general configuration.")
	opts.NoOp = pflag.Bool("noop", false, "Gather all required information and report on it, but do not create events in Mobilizòn.")
	opts.Register = pflag.Bool("register", false, "Register this bot and quit. A client id and client secret will be output.")
	opts.Authorize = pflag.Bool("authorize", false, "Authorize this bot and quit. An auth token and renew token will be output.")
	opts.Draft = pflag.Bool("draft", false, "Create events in draft mode.")

	pflag.Parse()

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

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: auth.AccessToken},
	)
	HttpClient = oauth2.NewClient(context.Background(), src)
	Client = graphql.NewClient("https://mobilisons.ch/api", HttpClient)

	// this will hold our json object whether local or from ConcertCloud
	var responseObject Response

	if *opts.File != "" {
		log.Println("using local file:", *opts.File)
		dat, err := os.ReadFile(*opts.File)
		if err != nil {
			log.Fatal(err)
		}
		json.Unmarshal(dat, &responseObject)
	} else {
		// Fetch some concerts from Concert Cloud
		fetchUrl := fmt.Sprintf("%s?%s", "https://api.concertcloud.live/api/events", ccQuery)
		response, err := http.Get(fetchUrl)
		if err != nil {
			fmt.Print(err.Error())
			os.Exit(1)
		}

		responseData, err := io.ReadAll(response.Body)
		if err != nil {
			log.Fatal(err)
		}

		json.Unmarshal(responseData, &responseObject)
	}

	var addrs = fetchAddrs(responseObject)

	createEvents(responseObject, addrs)
}

func fetchAddrs(responseObject Response) map[string]AddressInput {
	var addrs = make(map[string]AddressInput)

	for _, event := range responseObject.Event {

		// log.Println(fmt.Sprintf("Searching for: %s", event.Location))

		// if we already have the don't bother with the query
		_, ok := addrs[event.Location]
		if ok {
			// log.Println("Skipping " + event.Location)
			continue
		}

		query := fetchOSMAddr(event)
		// log.Println("Query from OSM:", query)

		var s struct {
			SearchAddress []AddressInput `graphql:"searchAddress(query: $query)"`
		}
		vars := map[string]interface{}{
			"query": query,
		}
		err := Client.Query(context.Background(), &s, vars)
		if err != nil {
			log.Println("fetchAddrs", err)
			time.Sleep(3 * time.Second)
			Client.Query(context.Background(), &s, vars)
		}

		if len(s.SearchAddress) == 0 {
			log.Println(fmt.Sprintf("Not found: %s", query))
		} else if len(s.SearchAddress) == 1 {
			a := s.SearchAddress[0]
			// log.Println("Mobilizòn returned: '" + a.Description + " " + a.Street + " " + a.Locality)
			addrs[event.Location] = a
			continue
		}

		for _, a := range s.SearchAddress {
			// log.Println("Mobilizòn returned: '" + a.Description + " " + a.Street + " " + a.Locality + " for " + event.Location + " " + event.City)
			if a.Description == event.Location && a.Locality == event.City {
				addrs[event.Location] = a
				break
			}
			addrs[event.Location] = s.SearchAddress[len(s.SearchAddress)-1]
		}
	}

	return addrs
}

func fetchOSMAddr(event Event) string {

	var addr Place

	// log.Println("Doing lookup in OpenStreetMap")
	var querystring = fmt.Sprintf("amenity=%s&city=%s&format=json&addressdetails=1",
		url.QueryEscape(event.Location),
		url.QueryEscape(event.City))
	var nurl = fmt.Sprintf("%s?%s", NominatumBaseURL, querystring)
	nresp, err := http.Get(nurl)

	if err != nil {
		log.Fatal(err.Error())
	}

	addrData, err := io.ReadAll(nresp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var addrObject NominatumResponse
	json.Unmarshal(addrData, &addrObject)

	if len(addrObject) == 0 {
		log.Println(fmt.Sprintf("OSM Place Not found: %s, %s", event.Location, event.City))
		return event.Location + " " + event.City
	} else if len(addrObject) == 1 {
		addr = addrObject[0]
	} else {
		for _, p := range addrObject {
			if p.Type == "nightclub" || p.Type == "bar" || p.Type == "restaurant" || p.Type == "theatre" || p.Type == "cinema" || p.Type == "arts_centre" {
				// log.Println("Addr Type:", p.Type)
				addr = p
				break
			}
		}
	}

	return event.Location + " " + addr.Address.Road + " " + addr.Address.City
}

func createEvents(r Response, addrs map[string]AddressInput) {
	var m struct {
		CreateEvent struct {
			Id   string
			Uuid string
		} `graphql:"createEvent(organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, category: $category, visibility: $visibility, description: $description, physicalAddress: $physicalAddress, beginsOn: $beginsOn, endsOn: $endsOn, draft: $draft, onlineAddress: $onlineAddress, externalParticipationUrl: $externalParticipationUrl, tags: $tags, joinOptions: $joinOptions, options: $options, picture: $picture)"`
	}

	var m_nopic struct {
		CreateEvent struct {
			Id   string
			Uuid string
		} `graphql:"createEvent(organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, category: $category, visibility: $visibility, description: $description, physicalAddress: $physicalAddress, beginsOn: $beginsOn, endsOn: $endsOn, draft: $draft, onlineAddress: $onlineAddress, externalParticipationUrl: $externalParticipationUrl, tags: $tags, joinOptions: $joinOptions, options: $options)"`
	}

	for _, event := range r.Event {

		// Do not upload events from bejazz.ch. They don't like us.
		// opt out FIXME this should be loaded from a file or something
		match, _ := regexp.MatchString("bejazz.ch", event.URL)
		if match {
			log.Println("Skipping BeJazz.")
			continue
		}

		event.Title = strings.TrimSpace(event.Title)
		// fmt.Println(event.Title)

		// hack to fix short titles
		if len(event.Title) < 3 {
			event.Title = event.Title + " ..."
		}

		if eventExists(event) {
			continue
		}

		var addr = addrs[event.Location]

		var tags = []string{
			"concert",
			event.Location,
			event.City + " Concerts",
			addr.Country + " Concerts/Konzerte",
		}

		tz := Timezone(*opts.Timezone)

		options := EventOptionsInput{
			CommentModeration: EventCommentModeration("ALLOW_ALL"),
			ShowStartTime:     graphql.Boolean(true),
			ShowEndTime:       graphql.Boolean(false),
			Timezone:          tz,
		}

		// add a plug for ConcertCloud
		event.Comment = event.Comment + " <br/><br/> " + CC_PLUG

		// get the event image
		var imageURL = event.ImageUrl

		// fetch the opengraph image for the event if there is no event image
		if imageURL == "" {
			imageURL = fetchOGImage(event.URL)
		}

		// fetch a backup image if we don't already have something
		if imageURL == "" {
			imageURL = fetchEventImage(event.URL)
		}

		if imageURL == "" {
			log.Println("No image found for " + event.URL)
		}

		var imageId string = ""
		// download the image
		if imageURL != "" {
			path, err := downloadFile(imageURL)
			if err != nil {
				log.Println(err)
			} else if *opts.NoOp {
				// log.Println("NoOp: Skipping the media upload too.")
			} else {
				// upload the image
				multi, err := newfileUploadRequest(path)
				if err != nil {
					log.Println(event.Title, " ", err)
				} else {
					// log.Println("Uploading the image")
					response, err := HttpClient.Do(multi)
					if err != nil {
						log.Println(err)
					}
					responseData, err := io.ReadAll(response.Body)
					if err != nil {
						log.Fatal(err)
					}
					var mediaObject MediaResponse
					json.Unmarshal(responseData, &mediaObject)
					imageId = mediaObject.Data.Upload.Id
				}
			}
		}

		// if the goskyr config has Mobilizòn event types use them
		var category = EventCategory("MUSIC")
		if slices.Contains(EventTypeStrings, event.Type) {
			category = EventCategory(event.Type)
		}

		// set up the query vars
		variables := map[string]interface{}{
			"organizerActorId":         graphql.ID(*opts.ActorID),
			"attributedToId":           graphql.ID(*opts.GroupID),
			"category":                 category,
			"visibility":               EventVisibility("PUBLIC"),
			"joinOptions":              EventJoinOptions("EXTERNAL"),
			"title":                    event.Title,
			"description":              event.Comment,
			"physicalAddress":          addr,
			"beginsOn":                 DateTime(event.Date.Format(time.RFC3339)),
			"endsOn":                   DateTime(event.Date.Add(time.Hour * 2).Format(time.RFC3339)),
			"draft":                    graphql.Boolean(*opts.Draft),
			"onlineAddress":            event.URL,
			"externalParticipationUrl": event.URL,
			"tags":                     tags,
			"options":                  options,
		}

		if imageId != "" {
			mi := MediaInput{MediaId: graphql.ID(imageId)}
			variables["picture"] = mi
		}

		if *opts.NoOp {

			// if this is a dry run just print some stuff out
			// spew.Dump(variables)
			// spew.Dump(imageURL)

		} else {

			if imageId == "" {
				// run the mutation against the Mobilizon instance
				err := Client.Mutate(context.Background(), &m_nopic, variables)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				// run the mutation against the Mobilizon instance
				err := Client.Mutate(context.Background(), &m, variables)
				if err != nil {
					log.Fatal(err)
				}
			}

			// calculate a hash of the event
			hash, err := rxhash.HashStruct(event)
			if err != nil {
				log.Fatal(err)
			}

			// output the hash and event ID to be stored somehwere
			fmt.Printf("%s %s %s\n", hash, m.CreateEvent.Id, m.CreateEvent.Uuid)
		}
	}
}

func registerApp() {

	type Registration struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}

	var posturl = "https://mobilisons.ch/apps"
	body := []byte(`name=Concert%20Cloud%20Bot&redirect_uri=https://login.microsoftonline.com/common/oauth2/nativeclient&website=https://concertcloud.live&scope=write:event:create%20write:media:upload`)
	r, err := http.NewRequest("POST", posturl, bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}

	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	c := &http.Client{}
	res, err := c.Do(r)
	if err != nil {
		log.Fatal(err)
	}

	resData, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	var reg Registration
	json.Unmarshal(resData, &reg)

	os.Setenv("GRAPHQL_CLIENT_ID", reg.ClientID)
	os.Setenv("GRAPHQL_CLIENT_SECRET", reg.ClientSecret)

	fmt.Println("export GRAPHQL_CLIENT_ID=" + reg.ClientID)
	fmt.Println("export GRAPHQL_CLIENT_SECRET=" + reg.ClientSecret)
}

func authorizeApp() {
	// Let's first check for a valid refreshToken in our config
	// If that doesn't work then we need to authorize interactively
	err := refreshAuthorization()
	if err == nil {
		return
	}

	var posturl = "https://mobilisons.ch/login/device/code"
	clientID := os.Getenv("GRAPHQL_CLIENT_ID")

	body := []byte("client_id=" + clientID + "&scope=write:event:create%20write:media:upload")
	r, err := http.NewRequest("POST", posturl, bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}

	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	c := &http.Client{}
	res, err := c.Do(r)
	if err != nil {
		log.Fatal(err)
	}

	resData, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	type DeviceCodeGrant struct {
		DeviceCode      string `json:"device_code"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
	}

	var resp DeviceCodeGrant
	json.Unmarshal(resData, &resp)

	fmt.Println("Please visit this URL and enter the code below " + resp.VerificationURI)
	fmt.Println()
	fmt.Println(resp.UserCode)
	fmt.Println()
	fmt.Println("Then press any key to continue.")

	// wait for input
	fmt.Scanln()

	var token_url = "https://mobilisons.ch/oauth/token"
	token_body := []byte("client_id=" + clientID + "&device_code=" + resp.DeviceCode + "&grant_type=urn:ietf:params:oauth:grant-type:device_code")
	tokreq, err := http.NewRequest("POST", token_url, bytes.NewBuffer(token_body))
	if err != nil {
		log.Fatal(err)
	}

	tokreq.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	tokres, err := c.Do(tokreq)

	resData, err = io.ReadAll(tokres.Body)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(*opts.AuthConfig, resData, 0600)

}

func fetchOGImage(url string) string {

	retUrl := ""

	// get the ogp object
	ogp, err := opengraph.Fetch(url)
	if err != nil {
		log.Println("fetchOGImage", err)
	}

	// convert URLs to absolute
	ogp.ToAbsURL()

	// if we have a URL return it
	if len(ogp.Image) > 0 && url != ogp.Image[0].URL+"/" {
		// but check that it works first
		res, err := http.Head(ogp.Image[0].URL)
		if err != nil {
			log.Println("fetchOGImage", err)
			return ""
		}
		if res.StatusCode == 200 {
			retUrl = ogp.Image[0].URL
		}
		// some venues put the full size image in the metadata
		if res.ContentLength > MAX_IMG_SIZE {
			retUrl = ""
		}
	}

	return retUrl
}

// this should try harder to find the best image
func fetchEventImage(url string) string {

	// log.Println("Fetching an image URL from " + url)
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

	// Is biggest best? Well maybe not, but that's what we have to work with.
	if len(srcs) > 0 {
		var best = 0
		var size int64 = 0
		for i, src := range srcs {
			// log.Println(src)
			// occassionally we get an inline image
			if strings.HasPrefix(src, "data:") {
				continue
			}
			// sometimes we don't get the absolute URL
			if strings.HasPrefix(src, "/") {
				continue
			}
			res, err := http.Head(src)
			if err != nil {
				log.Println("fetchEventImage", err)
			}
			cl := res.ContentLength
			if cl > size && cl < MAX_IMG_SIZE {
				best = i
				size = cl
			}
			// log.Printf("i: %d - size: %d cl: %d best: %d", i, size, cl, best)
		}
		return srcs[best]
	} else {
		return ""
	}
}

func newfileUploadRequest(path string) (*http.Request, error) {

	var fileContents []byte
	var fi fs.FileInfo
	if strings.HasPrefix(path, "data:") {
		// log.Println(path)
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

	// writer.SetBoundary("---------------------------164507724316293925132493775707")

	// TODO make this a template string or something to avoid the long line
	writer.WriteField("query", "mutation uploadMedia($file: Upload!, $name: String!) { uploadMedia(file: $file, name: $name) { id } }")
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

	r, err := http.NewRequest("POST", "https://mobilisons.ch/api", body)
	r.Header.Add("Content-Type", writer.FormDataContentType())

	return r, err
}

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
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return f.Name(), err
	}

	return f.Name(), nil
}

func eventExists(e Event) bool {

	// log.Println("Searching for '" + e.Title + "' " + e.Date.Format(time.RFC3339))
	var s struct {
		SearchEvents struct {
			Total    int `json:"total"`
			Elements []struct {
				Id       graphql.ID `json:"id"`
				Uuid     string     `json:"uuid"`
				Title    string     `json:"title"`
				BeginsOn string     `json:"beginsOn"`
			}
		} `graphql:"searchEvents(term: $term, beginsOn: $beginsOn)"`
	}
	vars := map[string]interface{}{
		"term":     e.Title,
		"beginsOn": DateTime(e.Date.Format(time.RFC3339)),
	}
	err := Client.Query(context.Background(), &s, vars)
	if err != nil {
		log.Println("Error checking if event exists:", err)
		//
		// FIXME
		//
		// When the server is loaded the graphql API fails to return
		// certain events. I haven't been able to identify why, but the
		// same events always fail which suggests that there must be a
		// better approach.
		//
		// That said, this works for the time being.
		//
		time.Sleep(3 * time.Second)
		Client.Query(context.Background(), &s, vars)
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
			"uuid": UUID(el.Uuid),
		}
		err := Client.Query(context.Background(), &f, fvars)
		if err != nil {
			log.Println("Failed fetching event by uuid:", el.Uuid, err)
		}

		if e.URL == f.Event.OnlineAddress {
			log.Println("Found event matching:", e.URL)
			// we have a match
			// FIXME update the title if it has changed
			return true
		}

	}

	log.Println("Event not found '" + e.Title + "' " + e.Date.Format(time.RFC3339) + " " + e.Location)
	return false
}

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
		log.Println(err)
	}
	err = json.Unmarshal(dat, &auth)
	if err != nil {
		log.Println(err)
	}

	// log.Println("Using refresh token: " + auth.RefreshToken)
	variables := map[string]interface{}{
		"rt": auth.RefreshToken,
	}

	// run the refresh token query. We need to resturn any errors from here
	// down because they mean that the refresh has failed and so we'll need
	// to do the regular authorization
	c := graphql.NewClient("https://mobilisons.ch/api", nil)
	err = c.Mutate(context.Background(), &m, variables)
	if err != nil {
		log.Println("Failed auth token renewal")
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
