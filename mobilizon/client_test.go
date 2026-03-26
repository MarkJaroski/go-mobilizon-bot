// mobilizon/client_test.go
package mobilizon

import (
	"context"

	"github.com/Khan/genqlient/graphql"
)

// MockGraphQLClient for testing
type MockGraphQLClient struct {
	MakeRequestFunc func(
		ctx context.Context,
		req *graphql.Request,
		resp *graphql.Response,
	) error
}

func (m *MockGraphQLClient) MakeRequest(
	ctx context.Context,
	req *graphql.Request,
	resp *graphql.Response,
) error {
	if m.MakeRequestFunc != nil {
		return m.MakeRequestFunc(ctx, req, resp)
	}
	return nil
}

// func TestClient_CreateEvent(t *testing.T) {
// 	mock := &MockGraphQLClient{
// 		MakeRequestFunc: func(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
// 			// Simulate successful response
// 			resp.Data = map[string]interface{}{
// 				"createEvent": map[string]interface{}{
// 					"id":    "123",
// 					"uuid":  "test-uuid",
// 					"title": "Test Event",
// 				},
// 			}
// 			return nil
// 		},
// 	}
//
// 	client := &Client{gqlClient: mock}
//
// 	// Test using generated function
// 	// ...
// }
