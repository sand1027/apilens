// Package graphqlop parses GraphQL HTTP payloads and SDL documents so
// discovery, watch, generate, and the test DSL share one interpretation of
// "what operation is this?".
package graphqlop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// Payload is the standard GraphQL-over-HTTP JSON body.
type Payload struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
}

// Operation is one parsed GraphQL operation from a query document.
type Operation struct {
	Type       string   // "query", "mutation", or "subscription"
	Name       string   // optional operation name (Login, Centers, ...)
	RootFields []string // top-level selection names (login, centers, ping)
}

// HTTPMethod is always POST for GraphQL-over-HTTP (Apollo's default for Stance).
const HTTPMethod = domain.Method("POST")

const (
	TypeQuery        = "query"
	TypeMutation     = "mutation"
	TypeSubscription = "subscription"
)

// DisplayMethod maps a GraphQL operation type to the registry method shown
// by `apilens list` (QUERY / MUTATION / SUBSCRIPTION). The HTTP runner still
// sends POST — see HTTPMethodFor.
func DisplayMethod(opType string) domain.Method {
	switch strings.ToLower(strings.TrimSpace(opType)) {
	case TypeMutation:
		return "MUTATION"
	case TypeSubscription:
		return "SUBSCRIPTION"
	default:
		return "QUERY"
	}
}

// HTTPMethodFor maps a registry/display method back to the HTTP verb to send.
// QUERY/MUTATION/SUBSCRIPTION all become POST; anything else is unchanged.
func HTTPMethodFor(m domain.Method) domain.Method {
	switch domain.NormalizeMethod(string(m)) {
	case "QUERY", "MUTATION", "SUBSCRIPTION":
		return HTTPMethod
	default:
		return domain.NormalizeMethod(string(m))
	}
}

// RegistryPath is the canonical discovered path for an operation field,
// e.g. /graphql/query/ping. Unique per (type, field) so QUERY ping and
// MUTATION ping cannot collapse in the registry.
func RegistryPath(opType, field string) string {
	t := strings.ToLower(strings.TrimSpace(opType))
	if t == "" {
		t = TypeQuery
	}
	f := strings.TrimSpace(field)
	if f == "" {
		f = "anonymous"
	}
	return "/graphql/" + t + "/" + f
}

// LooksLike reports whether body is a GraphQL-over-HTTP JSON payload.
func LooksLike(body []byte) bool {
	p, ok := Decode(body)
	return ok && strings.TrimSpace(p.Query) != ""
}

// Decode reads a GraphQL HTTP JSON body. ok is false if it isn't one.
func Decode(body []byte) (Payload, bool) {
	if len(body) == 0 {
		return Payload{}, false
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return Payload{}, false
	}
	query, _ := raw["query"].(string)
	if strings.TrimSpace(query) == "" {
		return Payload{}, false
	}
	p := Payload{Query: query}
	if name, ok := raw["operationName"].(string); ok {
		p.OperationName = name
	}
	if vars, ok := raw["variables"].(map[string]any); ok {
		p.Variables = vars
	}
	return p, true
}

// Encode builds a GraphQL HTTP JSON body.
func Encode(query string, variables any, operationName string) ([]byte, error) {
	body := map[string]any{"query": query}
	if operationName != "" {
		body["operationName"] = operationName
	}
	if variables != nil {
		body["variables"] = variables
	}
	return json.Marshal(body)
}

// ParseQuery extracts the first operation from a GraphQL document string.
func ParseQuery(query string) (Operation, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return Operation{}, fmt.Errorf("empty GraphQL query")
	}
	doc, err := parser.ParseQuery(&ast.Source{Name: "query.graphql", Input: query})
	if err != nil {
		return Operation{}, err
	}
	if doc == nil || len(doc.Operations) == 0 {
		return Operation{}, fmt.Errorf("GraphQL document has no operations")
	}
	op := doc.Operations[0]
	out := Operation{
		Type: strings.ToLower(string(op.Operation)),
		Name: op.Name,
	}
	if out.Type == "" {
		out.Type = TypeQuery
	}
	for _, sel := range op.SelectionSet {
		if f, ok := sel.(*ast.Field); ok {
			out.RootFields = append(out.RootFields, f.Name)
		}
	}
	return out, nil
}

// ParseHTTPBody decodes a GraphQL HTTP payload and parses its query.
func ParseHTTPBody(body []byte) (Payload, Operation, bool) {
	p, ok := Decode(body)
	if !ok {
		return Payload{}, Operation{}, false
	}
	op, err := ParseQuery(p.Query)
	if err != nil {
		return p, Operation{}, false
	}
	if p.OperationName != "" && op.Name == "" {
		op.Name = p.OperationName
	}
	return p, op, true
}

// PrimaryField is the first root field, used as the registry identity.
func (o Operation) PrimaryField() string {
	if len(o.RootFields) > 0 {
		return o.RootFields[0]
	}
	if o.Name != "" {
		return o.Name
	}
	return "anonymous"
}

// Label is the short name shown in watch/history: the operation name if
// the client sent one, otherwise the primary root field.
func (o Operation) Label() string {
	if o.Name != "" {
		return o.Name
	}
	return o.PrimaryField()
}

// DisplayColumns is the method + resource printed by `watch` / `history list`.
// GraphQL POST /graphql becomes QUERY ping / MUTATION login so captures are
// distinguishable. Non-GraphQL exchanges keep the HTTP method and URL.
func DisplayColumns(method domain.Method, rawURL string, reqBody []byte) (string, string) {
	if _, op, ok := ParseHTTPBody(reqBody); ok {
		return string(DisplayMethod(op.Type)), op.Label()
	}
	return string(method), rawURL
}

// ResponseHasErrors reports a GraphQL-over-HTTP errors[] array. HTTP 200
// with errors is a failed operation and should not look like REST success.
func ResponseHasErrors(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var env struct {
		Errors []any `json:"errors"`
	}
	if json.Unmarshal(body, &env) != nil {
		return false
	}
	return len(env.Errors) > 0
}
