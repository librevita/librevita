package fhir

// OperationOutcome is FHIR R4 OperationOutcome.
type OperationOutcome struct {
	ResourceType string                  `json:"resourceType"`
	Issue        []OperationOutcomeIssue `json:"issue"`
}

// OperationOutcomeIssue is OperationOutcome.issue.
type OperationOutcomeIssue struct {
	Severity    string `json:"severity"`
	Code        string `json:"code"`
	Diagnostics string `json:"diagnostics,omitempty"`
}

// Outcome builds a single-issue OperationOutcome.
func Outcome(severity, code, diagnostics string) OperationOutcome {
	return OperationOutcome{
		ResourceType: "OperationOutcome",
		Issue: []OperationOutcomeIssue{{
			Severity:    severity,
			Code:        code,
			Diagnostics: diagnostics,
		}},
	}
}
