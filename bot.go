package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/hasura/go-graphql-client"
	"github.com/rxwycdh/rxhash"
	"golang.org/x/oauth2"

	"github.com/spf13/pflag"
)

const CC_PLUG = "Help promote your favourite venues with: https://concertcloud.live/contribute"

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
	Date      time.Time `json:"date"`
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
}

var actorID *string
var groupID *string

func main() {
	ccCity := pflag.String("city", "X", "The concertcloud API param 'city'") // defaults to X to avoid flooding
	ccCountry := pflag.String("country", "", "The concertcloud API param 'country'")
	ccLimit := pflag.String("limit", "", "The concertcloud API param 'limit'")
	ccPage := pflag.String("page", "", "The concertcloud API param 'page'")
	ccRadius := pflag.String("radius", "", "The concertcloud API param 'radius'")
	ccDate := pflag.String("date", "", "The concertcloud API param 'date'")
	actorID = pflag.String("actor", "", "The Mobilizon actor ID to use as the event organizer.")
	groupID = pflag.String("group", "", "The Mobilizon group ID to use for the event attribution.")

	pflag.Parse()

	ccQuery := ""
	if *ccCity != "" {
		ccQuery = fmt.Sprintf("%s&city=%s", ccQuery, url.QueryEscape(*ccCity))
	}
	if *ccCountry != "" {
		ccQuery = fmt.Sprintf("%s&country=%s", ccQuery, url.QueryEscape(*ccCountry))
	}
	if *ccLimit != "" {
		ccQuery = fmt.Sprintf("%s&limit=%s", ccQuery, *ccLimit)
	}
	if *ccPage != "" {
		ccQuery = fmt.Sprintf("%s&page=%s", ccQuery, *ccPage)
	}
	if *ccRadius != "" {
		ccQuery = fmt.Sprintf("%s&radius=%s", ccQuery, *ccRadius)
	}
	if *ccDate != "" {
		ccQuery = fmt.Sprintf("%s&date=%s", ccQuery, url.QueryEscape(*ccDate))
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

		place, ok := addrs[event.Location]

		if ok {
			log.Println(fmt.Sprintf("Found : %s", place.DisplayName))
		} else {
			log.Println("Doing lookup in OpenStreetMap")
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
		} `graphql:"createEvent(organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, category: $category, visibility: $visibility, description: $description, physicalAddress: $physicalAddress, beginsOn: $beginsOn, endsOn: $endsOn, draft: $draft, onlineAddress: $onlineAddress, tags: $tags, joinOptions: $joinOptions, options: $options)"`
	}

	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GRAPHQL_TOKEN")},
	)
	httpClient := oauth2.NewClient(context.Background(), src)

	c := graphql.NewClient("https://mobilisons.ch/api", httpClient)

	for _, event := range r.Event {

		fmt.Println(event.Title)

		var place = addrs[event.Location]
		addr := AddressInput{
			Description: place.Name,
			Locality:    place.Address.City,
			PostalCode:  place.Address.PostCode,
			Street:      fmt.Sprintf("%s %s", place.Address.Road, place.Address.HouseNumber),
			Country:     place.Address.Country,
			Geom:        fmt.Sprintf("%s;%s", place.Lon, place.Lat),
		}

		// TODO fetch a picture

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
		}

		// add a plug for ConcertCloud
		event.Comment = fmt.Sprintf("%s\n\n%s", event.Comment, CC_PLUG)

		variables := map[string]interface{}{
			"organizerActorId": graphql.ID(*actorID),
			"attributedToId":   graphql.ID(*groupID),
			"category":         EventCategory("MUSIC"),
			"visibility":       EventVisibility("PUBLIC"),
			"joinOptions":      EventJoinOptions("FREE"),
			"title":            event.Title,
			"description":      event.Comment,
			"physicalAddress":  addr,
			"beginsOn":         DateTime(event.Date.Format(time.RFC3339)),
			"endsOn":           DateTime(event.Date.Add(time.Hour * 2).Format(time.RFC3339)),
			"draft":            true,
			"onlineAddress":    event.URL,
			"tags":             tags,
			"options":          options,
		}

		// TODO do authentication and fetch an organizerActorId

		// run the mutation against the Mobilizon instance
		err := c.Mutate(context.Background(), &m, variables)
		if err != nil {
			log.Fatal(err)
		}

		// calculate a hash of the event
		hash, err := rxhash.HashStruct(event)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s %s %s\n", hash, m.CreateEvent.Id, m.CreateEvent.Uuid)
	}
}
