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
	City      *string
	Country   *string
	Limit     *string
	Page      *string
	Radius    *string
	Date      *string
	ActorID   *string
	GroupID   *string
	Timezone  *string
	NoOp      *bool
	Register  *bool
	Authorize *bool
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
	Address     Address `json:"address"`
	DisplayName string  `json:"display_name"`
}

type NominatumResponse []Place

type AddressInput struct {
	Description string `json:"description"`
	Locality    string `json:"locality"`
	PostalCode  string `json:"postalCode"`
	Street      string `json:"street"`
	Country     string `json:"country"`
	Geom        string `json:"geom"`
}

type MediaInput struct {
	MediaId graphql.ID `json:"mediaId"`
}

var NominatumBaseURL = "https://nominatim.openstreetmap.org/search"

type EventCategory string

const (
	MUSIC   EventCategory = "MUSIC"
	PARTY   EventCategory = "PARTY"
	COMEDY  EventCategory = "COMEDY"
	THEATRE EventCategory = "THEATRE"
)

type EventVisibility string

const (
	PRIVATE    EventVisibility = "PRIVATE"
	PUBLIC     EventVisibility = "PUBLIC"
	RESTRICTED EventVisibility = "RESTRICTED"
	UNLISTED   EventVisibility = "UNLISTED"
)

type EventJoinOptions string

const (
	FREE EventJoinOptions = "FREE"
)

type DateTime string

type EventCommentModeration string

const (
	ALLOW_ALL EventCommentModeration = "ALLOW_ALL"
	CLOSED    EventCommentModeration = "CLOSED"
	MODERATED EventCommentModeration = "MODERATED"
)

type EventOptionsInput struct {
	CommentModeration EventCommentModeration `json:"commentModeration"`
	ShowStartTime     graphql.Boolean        `json:"showStartTime"`
	ShowEndTime       graphql.Boolean        `json:"showEndTime"`
	Timezone          string                 `json:"timezone"`
}

var actorID *string
var groupID *string
var timezone *string

func main() {
	opts.City = pflag.String("city", "X", "The concertcloud API param 'city'") // defaults to X to avoid accidental flooding
	opts.Country = pflag.String("country", "", "The concertcloud API param 'country'")
	opts.Limit = pflag.String("limit", "", "The concertcloud API param 'limit'")
	opts.Page = pflag.String("page", "", "The concertcloud API param 'page'")
	opts.Radius = pflag.String("radius", "", "The concertcloud API param 'radius'")
	opts.Date = pflag.String("date", "", "The concertcloud API param 'date'")
	opts.ActorID = pflag.String("actor", "", "The Mobilizon actor ID to use as the event organizer.")
	opts.GroupID = pflag.String("group", "", "The Mobilizon group ID to use for the event attribution.")
	opts.Timezone = pflag.String("timezone", "EU/Zurich", "The timezone to use for the event attribution.")
	opts.NoOp = pflag.Bool("noop", false, "Gather all required information and report on it, but do not create events in Mobilizòn.")
	opts.Register = pflag.Bool("register", false, "Register this bot and quit. A client id and client secret will be output.")
	opts.Authorize = pflag.Bool("authorize", false, "Authorize this bot and quit. An auth token and renew token will be output.")

	pflag.Parse()

	if *opts.Register {
		registerApp()
		return
	}

	if *opts.Authorize {
		authorizeApp()
		return
	}

	ccQuery := ""
	if *opts.City != "" {
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

	var responseObject Response
	json.Unmarshal(responseData, &responseObject)

	var addrs = fetchAddrs(responseObject)

	createEvents(responseObject, addrs)
}

func fetchAddrs(responseObject Response) map[string]Place {
	var addrs = make(map[string]Place)

	for _, event := range responseObject.Event {

		_, ok := addrs[event.Location]

		if !ok {
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
				log.Println(fmt.Sprintf("Not found: %s", event.Location))
				addrs[event.Location] = Place{}
			} else {
				addrs[event.Location] = addrObject[0]
			}
		}
	}

	return addrs
}

func createEvents(r Response, addrs map[string]Place) {
	var m struct {
		CreateEvent struct {
			Id   string
			Uuid string
		} `graphql:"createEvent(organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, category: $category, visibility: $visibility, description: $description, physicalAddress: $physicalAddress, beginsOn: $beginsOn, endsOn: $endsOn, draft: $draft, onlineAddress: $onlineAddress, tags: $tags, joinOptions: $joinOptions, options: $options, picture: $picture)"`
	}

	var m_nopic struct {
		CreateEvent struct {
			Id   string
			Uuid string
		} `graphql:"createEvent(organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, category: $category, visibility: $visibility, description: $description, physicalAddress: $physicalAddress, beginsOn: $beginsOn, endsOn: $endsOn, draft: $draft, onlineAddress: $onlineAddress, tags: $tags, joinOptions: $joinOptions, options: $options)"`
	}

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("MOBILIZON_ACCESS_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)

	c := graphql.NewClient("https://mobilisons.ch/api", httpClient)

	for _, event := range r.Event {

		// fmt.Println(event.Title)

		if eventExists(event, c) {
			continue
		}

		var place = addrs[event.Location]
		addr := AddressInput{
			Description: place.Name,
			Locality:    place.Address.City,
			PostalCode:  place.Address.PostCode,
			Street:      fmt.Sprintf("%s %s", place.Address.Road, place.Address.HouseNumber),
			Country:     place.Address.Country,
			Geom:        fmt.Sprintf("%s;%s", place.Lon, place.Lat),
		}

		var tags = []string{
			"concert",
			event.Location,
			fmt.Sprintf("%s%s", event.City, "concerts"),
			fmt.Sprintf("%s%s", place.Address.Country, "concerts"),
		}

		options := EventOptionsInput{
			CommentModeration: EventCommentModeration("ALLOW_ALL"),
			ShowStartTime:     graphql.Boolean(true),
			ShowEndTime:       graphql.Boolean(false),
			Timezone:          *opts.Timezone,
		}

		// add a plug for ConcertCloud
		event.Comment = fmt.Sprintf("%s\n\n%s", event.Comment, CC_PLUG)

		// fetch the official image for the event
		imageURL := fetchOGImage(event.URL)
		// fetch a backup image
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
					response, err := httpClient.Do(multi)
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

		// hack to fix short titles
		if len(event.Title) < 3 {
			event.Title = event.Title + " ..."
		}

		variables := map[string]interface{}{
			"organizerActorId": graphql.ID(*opts.ActorID),
			"attributedToId":   graphql.ID(*opts.GroupID),
			"category":         EventCategory("MUSIC"),
			"visibility":       EventVisibility("PUBLIC"),
			"joinOptions":      EventJoinOptions("FREE"),
			"title":            event.Title,
			"description":      event.Comment,
			"physicalAddress":  addr,
			"beginsOn":         DateTime(event.Date.Format(time.RFC3339)),
			"endsOn":           DateTime(event.Date.Add(time.Hour * 2).Format(time.RFC3339)),
			"draft":            false,
			"onlineAddress":    event.URL,
			"tags":             tags,
			"options":          options,
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
				err := c.Mutate(context.Background(), &m_nopic, variables)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				// run the mutation against the Mobilizon instance
				err := c.Mutate(context.Background(), &m, variables)
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
	var posturl = "https://mobilisons.ch/login/device/code"

	clientID := os.Getenv("GRAPHQL_CLIENT_ID")
	// clientSecret := os.Getenv("GRAPHQL_CLIENT_SECRET")

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

	type TokenResponse struct {
		AccessToken           string `json:"access_token"`
		ExpiresIn             string `json:"expires_in"`
		RefreshToken          string `json:"refresh_token"`
		RefreshTokenExpiresIn string `json:"refresh_token_expires_in"`
		Scopes                string `json:"scopes"`
		TokenType             string `json:"token_type"`
	}

	var tokenResp TokenResponse
	json.Unmarshal(resData, &tokenResp)

	os.Setenv("MOBILIZON_ACCESS_TOKEN", tokenResp.AccessToken)
	os.Setenv("MOBILIZON_REFRESH_TOKEN", tokenResp.RefreshToken)

	fmt.Println("export MOBILIZON_ACCESS_TOKEN=" + tokenResp.AccessToken)
	fmt.Println("export MOBILIZON_REFRESH_TOKEN=" + tokenResp.RefreshToken)

}

func fetchOGImage(url string) string {

	retUrl := ""

	// get the ogp object
	ogp, err := opengraph.Fetch(url)
	if err != nil {
		log.Println(err)
	}

	// convert URLs to absolute
	ogp.ToAbsURL()

	// if we have a URL return it
	if len(ogp.Image) > 0 && url != ogp.Image[0].URL+"/" {
		// but check that it works first
		res, err := http.Head(ogp.Image[0].URL)
		if err != nil {
			log.Println(err)
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
				log.Println(err)
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

func eventExists(e Event, c *graphql.Client) bool {

	// log.Println("Searching for ", e.Title, " ", e.Date.Format(time.RFC3339))
	var s struct {
		SearchEvents struct {
			Total    int `json:"total"`
			Elements []struct {
				Id       graphql.ID `json:"id"`
				Title    string     `json:"title"`
				BeginsOn string     `json:"beginsOn"`
			}
		} `graphql:"searchEvents(term: $term)"`
	}
	vars := map[string]interface{}{
		"term": e.Title + " " + e.Location,
	}
	err := c.Query(context.Background(), &s, vars)
	if err != nil {
		log.Println(err)
	}

	// loop through the events and return true if we have a real match
	for _, el := range s.SearchEvents.Elements {
		// include beginsOn in conditional
		// TODO include place name in condition
		if el.Title == e.Title && el.BeginsOn == e.Date.Format(time.RFC3339) {
			log.Println("Found: ", el.Title, " ", el.BeginsOn)
			return true
		}
	}

	// log.Println("Event not found ", e.Title, " ", e.Date.Format(time.RFC3339))
	return false
}
