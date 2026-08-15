package domain

// Endpoint represents a discovered or registered API route. Discovery lands
// in v2; the type is defined now because pkg/apilens.Engine's interface
// shape is frozen early (docs/02-packages.md section 4, ADR-010).
type Endpoint struct {
	ID            EndpointID
	Method        Method
	Path          string
	Sources       []string
	PrimarySource string
	Tags          []string
	Spec          *EndpointSpec
}

// EndpointSpec holds the richer OpenAPI-derived detail. Empty for
// heuristically discovered or watch-observed endpoints.
type EndpointSpec struct {
	Name        string // operationId
	Parameters  []Parameter
	RequestBody any
	Responses   any
}

type Parameter struct {
	Name     string
	In       string // "path" | "query" | "header"
	Required bool
	Type     string
}
