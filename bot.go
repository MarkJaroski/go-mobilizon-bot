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
)

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

type DateTime string

func main() {
	// read the concertcloud API
	// TODO: this needs to be in configuration
	response, err := http.Get("https://api.concertcloud.live/api/events?title=&city=Lausanne&limit=1")
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
		} `graphql:"createEvent(organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, category: $category, visibility: $visibility, description: $description, physicalAddress: $physicalAddress, beginsOn: $beginsOn, draft: $draft, onlineAddress: $onlineAddress, tags: $tags)"`
	}

	c := graphql.NewClient("https://mobilisons.ch/api", nil)

	for _, event := range r.Event {

		fmt.Println(event.Title)

		var place = addrs[event.Location]
		addr := AddressInput{
			Description: place.Name,
			Locality:    place.Address.City,
			PostalCode:  place.Address.PostCode,
			Street:      fmt.Sprintf("%s %s", place.Address.Road, place.Address.HouseNumber),
			Country:     place.Address.Country,
		}

		// TODO fetch a picture

		var tags = []string{
			"concert",
			event.Location,
			fmt.Sprintf("%s%s", event.Location, "concerts"),
			fmt.Sprintf("%s%s", place.Address.Country, "concerts"),
		}

		variables := map[string]interface{}{
			"organizerActorId": graphql.ID("0"),
			"attributedToId":   graphql.ID("0"),
			"category":         EventCategory("MUSIC"),
			"visibility":       EventVisibility("PUBLIC"),
			"title":            event.Title,
			"description":      event.Comment,
			"physicalAddress":  addr,
			"beginsOn":         DateTime(event.Date.Format(time.RFC3339)),
			"draft":            true,
			"onlineAddress":    event.URL,
			"tags":             tags,
		}

		// TODO do authentication and fetch an organizerActorId

		err := c.Mutate(context.Background(), &m, variables)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Created event %s %s\n", m.CreateEvent.Id, m.CreateEvent.Uuid)
	}
}
