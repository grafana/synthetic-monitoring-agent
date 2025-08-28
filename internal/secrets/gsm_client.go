package secrets

import (
	"context"
	"fmt"
	"net/http"

	gsmClient "github.com/grafana/gsm-api-go-client"
)

// GSMClientFactory creates GSM clients with proper configuration
type GSMClientFactory struct{}

// NewGSMClientFactory creates a new GSM client factory
func NewGSMClientFactory() *GSMClientFactory {
	return &GSMClientFactory{}
}

// CreateClient creates a new GSM client with the provided URL and token
func (f *GSMClientFactory) CreateClient(url, token string) (gsmClient.ClientWithResponsesInterface, error) {
	if url == "" {
		return nil, fmt.Errorf("GSM URL cannot be empty")
	}
	if token == "" {
		return nil, fmt.Errorf("GSM token cannot be empty")
	}
	return gsmClient.NewClientWithResponses(url, withAuth(token), withAcceptJSON())
}

// withAuth adds the Authorization header with Bearer token
func withAuth(token string) gsmClient.ClientOption {
	return gsmClient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Add("Authorization", "Bearer "+token)
		return nil
	})
}

// withAcceptJSON adds the Accept: application/json header
func withAcceptJSON() gsmClient.ClientOption {
	return gsmClient.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
		req.Header.Add("Accept", "application/json")
		return nil
	})
}
