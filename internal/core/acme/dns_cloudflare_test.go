package acme

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudflareProvider_PresentAndCleanUp(t *testing.T) {
	ctx := context.Background()

	var recordCreated bool
	var recordDeleted bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-cf-token", r.Header.Get("Authorization"))

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/zones":
			// List zones
			res := cfResponse[[]cfZone]{
				Success: true,
				Result: []cfZone{
					{ID: "zone123", Name: "example.org"},
				},
			}
			_ = json.NewEncoder(w).Encode(res)

		case r.Method == http.MethodPost && r.URL.Path == "/zones/zone123/dns_records":
			// Create record
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.Equal(t, "TXT", payload["type"])
			assert.Equal(t, "_acme-challenge.example.org", payload["name"])
			assert.Equal(t, "txt-digest-val", payload["content"])
			recordCreated = true

			res := cfResponse[cfRecord]{
				Success: true,
				Result: cfRecord{
					ID:      "rec456",
					Type:    "TXT",
					Name:    "_acme-challenge.example.org",
					Content: "txt-digest-val",
				},
			}
			_ = json.NewEncoder(w).Encode(res)

		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone123/dns_records":
			// List records for cleanup
			res := cfResponse[[]cfRecord]{
				Success: true,
				Result: []cfRecord{
					{
						ID:      "rec456",
						Type:    "TXT",
						Name:    "_acme-challenge.example.org",
						Content: "txt-digest-val",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(res)

		case r.Method == http.MethodDelete && r.URL.Path == "/zones/zone123/dns_records/rec456":
			// Delete record
			recordDeleted = true
			res := cfResponse[map[string]string]{
				Success: true,
			}
			_ = json.NewEncoder(w).Encode(res)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := NewCloudflareProvider("test-cf-token", "")
	p.baseURL = server.URL
	p.client = server.Client()

	err := p.Present(ctx, "example.org", "txt-digest-val")
	require.NoError(t, err)
	assert.True(t, recordCreated)

	err = p.CleanUp(ctx, "example.org", "txt-digest-val")
	require.NoError(t, err)
	assert.True(t, recordDeleted)
}
