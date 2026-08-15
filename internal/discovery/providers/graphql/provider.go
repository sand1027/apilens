// Package graphql implements SDL-based GraphQL discovery. It never
// executes user code and never invents operations that are not declared
// on Query, Mutation, or Subscription.
package graphql

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/sandeepv/apilens/internal/discovery"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/graphqlop"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

var wellKnownNames = []string{
	"schema.graphql", "schema.gql",
	"schema.graphqls",
}

// Stance and similar monorepos nest SDL under apps/api/src/modules/...
const maxWalkDepth = 12

// Provider implements discovery.Provider for GraphQL SDL files.
type Provider struct {
	ConfigPaths []string
}

// New builds a GraphQL provider. configPaths comes from
// discovery.graphql.paths in config.yaml.
func New(configPaths []string) *Provider {
	return &Provider{ConfigPaths: configPaths}
}

func (p *Provider) Name() string { return "graphql" }

func (p *Provider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	files, err := p.candidateFiles(root)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

func (p *Provider) Discover(ctx context.Context, root fs.FS, opts discovery.Options) ([]domain.Endpoint, error) {
	explicit := len(opts.Paths) > 0
	files := opts.Paths
	if !explicit {
		var err error
		files, err = p.candidateFiles(root)
		if err != nil {
			return nil, err
		}
	}

	endpoints, parseErr := endpointsFromFiles(root, files, explicit)
	if explicit {
		if parseErr != nil {
			return nil, parseErr
		}
		return sortEndpoints(endpoints), nil
	}

	// A truncated or otherwise invalid merged schema.graphql must not
	// hide the per-module SDL next to it (Stance's root schema.graphql
	// currently ends mid-description). Fall back to a walk.
	if len(endpoints) == 0 {
		walked, err := discovery.WalkSourceFiles(root, []string{".graphql", ".gql", ".graphqls"}, discovery.DefaultIgnoreDirs, maxWalkDepth)
		if err != nil {
			return nil, err
		}
		var fallback []string
		seen := map[string]bool{}
		for _, f := range files {
			seen[f] = true
		}
		for _, f := range walked {
			if !seen[f] {
				fallback = append(fallback, f)
			}
		}
		more, ferr := endpointsFromFiles(root, fallback, false)
		if len(more) > 0 {
			endpoints = more
		} else if parseErr != nil {
			return nil, parseErr
		} else if ferr != nil {
			return nil, ferr
		}
	}

	return sortEndpoints(endpoints), nil
}

func endpointsFromFiles(root fs.FS, files []string, explicit bool) ([]domain.Endpoint, error) {
	var docs []*ast.SchemaDocument
	var firstErr error
	for _, f := range files {
		data, err := fs.ReadFile(root, f)
		if err != nil {
			if explicit {
				return nil, err
			}
			continue
		}
		doc, err := parser.ParseSchema(&ast.Source{Name: f, Input: string(data)})
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", f, err)
			}
			if explicit {
				return nil, firstErr
			}
			continue
		}
		docs = append(docs, doc)
	}
	if len(docs) == 0 {
		return nil, firstErr
	}
	return fieldsFromDocs(docs), nil
}

func fieldsFromDocs(docs []*ast.SchemaDocument) []domain.Endpoint {
	seen := map[string]bool{}
	var out []domain.Endpoint
	addDef := func(def *ast.Definition) {
		if def == nil || def.Kind != ast.Object {
			return
		}
		var opType string
		switch def.Name {
		case "Query":
			opType = graphqlop.TypeQuery
		case "Mutation":
			opType = graphqlop.TypeMutation
		case "Subscription":
			opType = graphqlop.TypeSubscription
		default:
			return
		}
		for _, ep := range fieldsOf(def, opType) {
			key := string(ep.Method) + " " + ep.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, ep)
		}
	}
	for _, doc := range docs {
		for _, def := range doc.Definitions {
			addDef(def)
		}
		for _, def := range doc.Extensions {
			addDef(def)
		}
	}
	return out
}

func fieldsOf(obj *ast.Definition, opType string) []domain.Endpoint {
	if obj == nil {
		return nil
	}
	var out []domain.Endpoint
	for _, f := range obj.Fields {
		if f == nil || strings.HasPrefix(f.Name, "__") {
			continue
		}
		// Schema placeholders like Query._empty are not real operations.
		if f.Name == "_empty" {
			continue
		}
		method := graphqlop.DisplayMethod(opType)
		p := graphqlop.RegistryPath(opType, f.Name)
		ep := domain.Endpoint{
			ID:            domain.NewEndpointID(method, p),
			Method:        method,
			Path:          p,
			Sources:       []string{"graphql"},
			PrimarySource: "graphql",
			Tags:          []string{"graphql", opType},
			Spec: &domain.EndpointSpec{
				Name: f.Name,
			},
		}
		for _, arg := range f.Arguments {
			ep.Spec.Parameters = append(ep.Spec.Parameters, domain.Parameter{
				Name:     arg.Name,
				In:       "graphql",
				Required: arg.Type != nil && arg.Type.NonNull,
				Type:     arg.Type.String(),
			})
		}
		out = append(out, ep)
	}
	return out
}

func sortEndpoints(endpoints []domain.Endpoint) []domain.Endpoint {
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Method != endpoints[j].Method {
			return endpoints[i].Method < endpoints[j].Method
		}
		return endpoints[i].Path < endpoints[j].Path
	})
	return endpoints
}

func (p *Provider) candidateFiles(root fs.FS) ([]string, error) {
	var found []string
	seen := map[string]bool{}
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			found = append(found, name)
		}
	}

	for _, cp := range p.ConfigPaths {
		if fileExists(root, cp) {
			add(cp)
		}
	}
	if len(found) > 0 {
		return found, nil
	}

	for _, name := range wellKnownNames {
		if fileExists(root, name) {
			add(name)
		}
		apiName := path.Join(".apilens", "api", name)
		if fileExists(root, apiName) {
			add(apiName)
		}
	}
	if len(found) > 0 {
		// A merged schema.graphql is complete; don't also load every
		// module file or fields duplicate. Discover falls back to a
		// walk if this file does not parse.
		return found, nil
	}

	walked, err := discovery.WalkSourceFiles(root, []string{".graphql", ".gql", ".graphqls"}, discovery.DefaultIgnoreDirs, maxWalkDepth)
	if err != nil {
		return nil, err
	}
	for _, f := range walked {
		add(f)
	}
	return found, nil
}

func fileExists(root fs.FS, name string) bool {
	info, err := fs.Stat(root, name)
	return err == nil && info != nil && !info.IsDir()
}
