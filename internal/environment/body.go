package environment

import (
	"encoding/json"

	"github.com/sandeepv/apilens/internal/domain"
)

// interpolateBody serializes a BodyTemplate into bytes + content type.
// JSON bodies have "{{var}}" placeholders expanded in every string leaf
// before marshaling; raw bodies are interpolated as a single string.
func (r *Resolver) interpolateBody(b domain.BodyTemplate) ([]byte, string, error) {
	if b.IsEmpty() {
		return nil, "", nil
	}
	if b.Raw != "" {
		val, err := r.Interpolate(b.Raw)
		if err != nil {
			return nil, "", err
		}
		ct := b.ContentType
		if ct == "" {
			ct = "text/plain"
		}
		return []byte(val), ct, nil
	}
	expanded, err := r.interpolateJSONValue(b.JSON, r.Interpolate)
	if err != nil {
		return nil, "", err
	}
	out, err := json.Marshal(expanded)
	if err != nil {
		return nil, "", domain.NewConfigError("marshaling request body", err)
	}
	return out, "application/json", nil
}

// interpolateJSONValue recursively expands every string leaf in a decoded
// JSON tree (map[string]any / []any / scalar) using interpolateStr. The
// interpolation function is a parameter — not always r.Interpolate —
// so the same tree-walk serves cleanup:'s db.<connection>.filter values,
// which need r.InterpolateAgainstResponse instead (see CleanupSpec's doc
// comment for why cleanup has its own placeholder form).
func (r *Resolver) interpolateJSONValue(v any, interpolateStr func(string) (string, error)) (any, error) {
	switch t := v.(type) {
	case string:
		return interpolateStr(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			expanded, err := r.interpolateJSONValue(val, interpolateStr)
			if err != nil {
				return nil, err
			}
			out[k] = expanded
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			expanded, err := r.interpolateJSONValue(val, interpolateStr)
			if err != nil {
				return nil, err
			}
			out[i] = expanded
		}
		return out, nil
	default:
		return t, nil
	}
}
