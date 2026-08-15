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
	expanded, err := r.interpolateJSONValue(b.JSON)
	if err != nil {
		return nil, "", err
	}
	out, err := json.Marshal(expanded)
	if err != nil {
		return nil, "", domain.NewConfigError("marshaling request body", err)
	}
	return out, "application/json", nil
}

func (r *Resolver) interpolateJSONValue(v any) (any, error) {
	switch t := v.(type) {
	case string:
		return r.Interpolate(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			expanded, err := r.interpolateJSONValue(val)
			if err != nil {
				return nil, err
			}
			out[k] = expanded
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			expanded, err := r.interpolateJSONValue(val)
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
