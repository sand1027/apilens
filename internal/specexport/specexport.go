// Package specexport builds an OpenAPI 3 document from the registry
// (docs/07-discovery.md's endpoint store) plus optional captured
// exchanges, implementing plan.md v7's "OpenAPI generation from registry
// + captures" and "apilens spec export writes an OpenAPI file the team
// can review". It reuses kin-openapi's openapi3 types (ADR-008: do not
// hand-roll OpenAPI semantics) so the written document is structurally
// correct and re-parseable by internal/discovery/providers/openapi.
package specexport

import (
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/sandeepv/apilens/internal/domain"
)

// Options configures Build.
type Options struct {
	Title   string // defaults to "ApiLens Export"
	Version string // defaults to "1.0.0"
	BaseURL string // optional servers[0].url hint (docs/07-discovery.md section 3: "hint only")
	// Examples maps an endpoint's normalized (method, path) identity —
	// domain.NewEndpointID — to a captured exchange whose response body
	// should be used to infer a response schema for that endpoint
	// (plan.md v7: "OpenAPI generation from registry + captures").
	Examples map[domain.EndpointID]domain.Exchange
}

// Build converts endpoints (typically from registry.Store.List) into an
// OpenAPI 3 document. Endpoints discovered via OpenAPI already carry rich
// Spec data (docs/07-discovery.md section 3's mapping table, inverted
// here); endpoints from Express or watch get a minimal operation plus
// whatever a matching capture in opts.Examples can infer.
func Build(endpoints []domain.Endpoint, opts Options) *openapi3.T {
	title := opts.Title
	if title == "" {
		title = "ApiLens Export"
	}
	version := opts.Version
	if version == "" {
		version = "1.0.0"
	}

	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: title, Version: version},
		Paths:   openapi3.NewPaths(),
	}
	if opts.BaseURL != "" {
		doc.Servers = openapi3.Servers{{URL: opts.BaseURL}}
	}

	// Group by canonical path so multiple methods on the same path share
	// one PathItem, mirroring how a hand-written OpenAPI doc looks.
	byPath := map[string][]domain.Endpoint{}
	var pathOrder []string
	for _, ep := range endpoints {
		p := domain.NormalizePath(ep.Path)
		if _, ok := byPath[p]; !ok {
			pathOrder = append(pathOrder, p)
		}
		byPath[p] = append(byPath[p], ep)
	}
	sort.Strings(pathOrder)

	for _, p := range pathOrder {
		item := &openapi3.PathItem{}
		group := byPath[p]
		sort.Slice(group, func(i, j int) bool { return group[i].Method < group[j].Method })
		for _, ep := range group {
			op := buildOperation(ep, opts.Examples)
			assignOperation(item, ep.Method, op)
		}
		doc.Paths.Set(p, item)
	}

	return doc
}

// assignOperation sets the right PathItem field for method — OpenAPI has
// one named field per HTTP method rather than a map, so this is a small
// switch rather than reflection.
func assignOperation(item *openapi3.PathItem, method domain.Method, op *openapi3.Operation) {
	switch string(method) {
	case "GET":
		item.Get = op
	case "POST":
		item.Post = op
	case "PUT":
		item.Put = op
	case "PATCH":
		item.Patch = op
	case "DELETE":
		item.Delete = op
	case "HEAD":
		item.Head = op
	case "OPTIONS":
		item.Options = op
	}
}

func buildOperation(ep domain.Endpoint, examples map[domain.EndpointID]domain.Exchange) *openapi3.Operation {
	op := &openapi3.Operation{Tags: ep.Tags}

	if ep.Spec != nil {
		op.OperationID = ep.Spec.Name
		for _, p := range ep.Spec.Parameters {
			op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
				Value: &openapi3.Parameter{
					Name:     p.Name,
					In:       p.In,
					Required: p.Required,
					Schema:   schemaRefForPrimitiveType(p.Type),
				},
			})
		}
	}

	statusCode := 200
	description := "Response"
	var bodySchema *openapi3.Schema

	if ex, ok := examples[domain.NewEndpointID(ep.Method, ep.Path)]; ok {
		if ex.Response.StatusCode != 0 {
			statusCode = ex.Response.StatusCode
		}
		if inferred := inferSchema(ex.Response.Body); inferred != nil {
			bodySchema = inferred
		}
	}

	resp := &openapi3.Response{Description: &description}
	if bodySchema != nil {
		resp.Content = openapi3.NewContentWithJSONSchema(bodySchema)
	}
	op.Responses = openapi3.NewResponses(openapi3.WithStatus(statusCode, &openapi3.ResponseRef{Value: resp}))

	return op
}

func schemaRefForPrimitiveType(t string) *openapi3.SchemaRef {
	switch t {
	case "integer":
		return openapi3.NewSchemaRef("", openapi3.NewInt64Schema())
	case "number":
		return openapi3.NewSchemaRef("", openapi3.NewFloat64Schema())
	case "boolean":
		return openapi3.NewSchemaRef("", openapi3.NewBoolSchema())
	default:
		return openapi3.NewSchemaRef("", openapi3.NewStringSchema())
	}
}
