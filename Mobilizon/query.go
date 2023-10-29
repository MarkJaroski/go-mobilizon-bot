package mobilizon

import (
	"context"

	gqlclient "git.sr.ht/~emersion/gqlclient"
)

func CreateEvent(client *gqlclient.Client, ctx context.Context, organizerActorId string, title string, attributedToId string, description string, beginsOn DateTime, endsOn *DateTime, status *EventStatus, visibility *EventVisibility, joinOptions *EventJoinOptions, draft *bool, tags []*string, picture *MediaInput, onlineAddress *string, phoneAddress *string, category *EventCategory, physicalAddress *AddressInput, options *EventOptionsInput, contacts []*Contact) (createEvent *Event, err error) {
	op := gqlclient.NewOperation("mutation createEvent ($organizerActorId: ID!, $title: String!, $attributedToId: ID, $description: String!, $beginsOn: DateTime!, $endsOn: DateTime, $status: EventStatus, $visibility: EventVisibility, $joinOptions: EventJoinOptions, $draft: Boolean, $tags: [String], $picture: MediaInput, $onlineAddress: String, $phoneAddress: String, $category: EventCategory, $physicalAddress: AddressInput, $options: EventOptionsInput, $contacts: [Contact]) {\n\tcreateEvent(organizerActorId: $organizerActorId, attributedToId: $attributedToId, title: $title, description: $description, beginsOn: $beginsOn, endsOn: $endsOn, status: $status, visibility: $visibility, joinOptions: $joinOptions, draft: $draft, tags: $tags, picture: $picture, onlineAddress: $onlineAddress, phoneAddress: $phoneAddress, category: $category, physicalAddress: $physicalAddress, options: $options, contacts: $contacts) {\n\t\tid\n\t\tuuid\n\t}\n}\n")
	op.Var("organizerActorId", organizerActorId)
	op.Var("title", title)
	op.Var("attributedToId", attributedToId)
	op.Var("description", description)
	op.Var("beginsOn", beginsOn)
	op.Var("endsOn", endsOn)
	op.Var("status", status)
	op.Var("visibility", visibility)
	op.Var("joinOptions", joinOptions)
	op.Var("draft", draft)
	op.Var("tags", tags)
	op.Var("picture", picture)
	op.Var("onlineAddress", onlineAddress)
	op.Var("phoneAddress", phoneAddress)
	op.Var("category", category)
	op.Var("physicalAddress", physicalAddress)
	op.Var("options", options)
	op.Var("contacts", contacts)
	var respData struct {
		CreateEvent *Event
	}
	err = client.Execute(ctx, op, &respData)
	return respData.CreateEvent, err
}
