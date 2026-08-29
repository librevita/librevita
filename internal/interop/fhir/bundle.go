package fhir

import "encoding/json"

// Bundle is FHIR R4 Bundle.
type Bundle struct {
	ResourceType string        `json:"resourceType"`
	ID           string        `json:"id,omitempty"`
	Type         string        `json:"type"`
	Timestamp    string        `json:"timestamp,omitempty"`
	Entry        []BundleEntry `json:"entry,omitempty"`
}

// BundleEntry is one Bundle.entry.
type BundleEntry struct {
	FullURL  string          `json:"fullUrl,omitempty"`
	Resource json.RawMessage `json:"resource,omitempty"`
	Request  *BundleRequest  `json:"request,omitempty"`
}

// BundleRequest is Bundle.entry.request for transactions.
type BundleRequest struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

// PeekType reads resourceType from a raw resource.
func PeekType(raw json.RawMessage) string {
	var peek struct {
		ResourceType string `json:"resourceType"`
	}
	_ = json.Unmarshal(raw, &peek)
	return peek.ResourceType
}
