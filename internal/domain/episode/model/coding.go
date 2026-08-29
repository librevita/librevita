package model

// Coding is a clinical code (system + code + display).
type Coding struct {
	System  string
	Code    string
	Display string
}

// Empty reports whether c has no system or code.
func (c Coding) Empty() bool {
	return c.System == "" && c.Code == ""
}
