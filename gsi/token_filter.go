package gsi

// Defines an API for token filters. A token filter decides, if a given auth token is acceptable for the server or if it
// should rather be rejected. The goal of a token filter is not syntax validation, but rather enforcing security
// constraints.
type TokenFilter interface {
	// Checks for a given token if a GSI server should accept it.
	Accept(authToken string) bool
}

type ToggleTokenFilter struct {
	Value bool
}

func (f *ToggleTokenFilter) Accept(string) bool {
	return f.Value
}
