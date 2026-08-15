package specexport

import (
	"encoding/json"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

// inferSchema builds a best-effort OpenAPI Schema from a captured JSON
// response body (plan.md v7: "OpenAPI generation from registry +
// captures"). Returns nil for a non-JSON or empty body — spec export
// still writes the endpoint without a response schema rather than
// guessing at one (mirrors docs/07-discovery.md's discipline of never
// inventing data not actually present).
func inferSchema(body []byte) *openapi3.Schema {
	if len(body) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return inferValue(parsed)
}

func inferValue(v any) *openapi3.Schema {
	switch t := v.(type) {
	case nil:
		return openapi3.NewSchema() // untyped/nullable — no concrete type to infer
	case bool:
		return openapi3.NewBoolSchema()
	case float64:
		// encoding/json decodes all JSON numbers as float64; treat whole
		// numbers as integers for a friendlier schema, matching how most
		// hand-written OpenAPI specs distinguish id-like fields.
		if t == float64(int64(t)) {
			return openapi3.NewInt64Schema()
		}
		return openapi3.NewFloat64Schema()
	case string:
		return openapi3.NewStringSchema()
	case []any:
		items := openapi3.NewSchema()
		if len(t) > 0 {
			items = inferValue(t[0])
		}
		arr := openapi3.NewArraySchema()
		arr.Items = openapi3.NewSchemaRef("", items)
		return arr
	case map[string]any:
		obj := openapi3.NewObjectSchema()
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic property order across runs
		for _, k := range keys {
			obj.WithProperty(k, inferValue(t[k]))
		}
		return obj
	default:
		return openapi3.NewSchema()
	}
}
