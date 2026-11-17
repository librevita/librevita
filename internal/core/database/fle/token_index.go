package fle

// NameTokenIndex stores the blind-indexed tokens of a personal name along with
// the key and algorithm header, separating metadata from the raw token digests.
type NameTokenIndex struct {
	Header string   `json:"h"`
	Tokens []string `json:"t"`
}
