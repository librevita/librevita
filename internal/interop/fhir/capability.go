package fhir

// CapabilityStatement is a minimal FHIR R4 CapabilityStatement.
type CapabilityStatement struct {
	ResourceType   string                    `json:"resourceType"`
	Status         string                    `json:"status"`
	Date           string                    `json:"date,omitempty"`
	Kind           string                    `json:"kind"`
	Software       *CapabilitySoftware       `json:"software,omitempty"`
	Implementation *CapabilityImplementation `json:"implementation,omitempty"`
	FhirVersion    string                    `json:"fhirVersion"`
	Format         []string                  `json:"format"`
	Rest           []CapabilityRest          `json:"rest,omitempty"`
}

// CapabilitySoftware is CapabilityStatement.software.
type CapabilitySoftware struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// CapabilityImplementation is CapabilityStatement.implementation.
type CapabilityImplementation struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

// CapabilityRest is CapabilityStatement.rest.
type CapabilityRest struct {
	Mode     string               `json:"mode"`
	Resource []CapabilityResource `json:"resource,omitempty"`
}

// CapabilityResource is CapabilityStatement.rest.resource.
type CapabilityResource struct {
	Type        string                  `json:"type"`
	Interaction []CapabilityInteraction `json:"interaction,omitempty"`
	SearchParam []CapabilitySearchParam `json:"searchParam,omitempty"`
	Operation   []CapabilityOperation   `json:"operation,omitempty"`
}

// CapabilityInteraction is rest.resource.interaction.
type CapabilityInteraction struct {
	Code string `json:"code"`
}

// CapabilitySearchParam is rest.resource.searchParam.
type CapabilitySearchParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CapabilityOperation is rest.resource.operation.
type CapabilityOperation struct {
	Name       string `json:"name"`
	Definition string `json:"definition,omitempty"`
}

// ServerCapability returns the LibreVita SOAP facade CapabilityStatement.
func ServerCapability(base string) CapabilityStatement {
	read := []CapabilityInteraction{{Code: "read"}}
	return CapabilityStatement{
		ResourceType: "CapabilityStatement",
		Status:       "active",
		Kind:         "instance",
		Software:     &CapabilitySoftware{Name: "LibreVita"},
		Implementation: &CapabilityImplementation{
			Description: "LibreVita SOAP document facade. POST Bundle writes a chart document, not a FHIR transaction.",
			URL:         base,
		},
		FhirVersion: FHIRVersion,
		Format:      []string{ContentType},
		Rest: []CapabilityRest{{
			Mode: "server",
			Resource: []CapabilityResource{
				{
					Type:        "Encounter",
					Interaction: append(read, CapabilityInteraction{Code: "search-type"}),
					SearchParam: []CapabilitySearchParam{{Name: "patient", Type: "reference"}},
				},
				{
					Type:        "Composition",
					Interaction: read,
					Operation: []CapabilityOperation{{
						Name:       "document",
						Definition: "http://hl7.org/fhir/OperationDefinition/Composition-document",
					}},
				},
				{
					Type:        "Bundle",
					Interaction: []CapabilityInteraction{{Code: "create"}},
				},
			},
		}},
	}
}
