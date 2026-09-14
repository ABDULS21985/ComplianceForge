package openapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

var operationIDPattern = regexp.MustCompile(`^[a-z][A-Za-z0-9]+$`)

// Route identifies one HTTP operation independently of its implementation.
type Route struct {
	Method string
	Path   string
}

func (r Route) String() string { return r.Method + " " + r.Path }

// Contract is a validated OpenAPI document plus its indexed operations.
type Contract struct {
	Document   *openapi3.T
	Operations map[Route]*openapi3.Operation
}

// Load validates the OpenAPI document, local references, operation IDs, and
// the repository's production-contract conventions. External references are
// deliberately disabled so CI never depends on mutable network content.
func Load(ctx context.Context, filename string) (*Contract, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false

	document, err := loader.LoadFromFile(filename)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI document: %w", err)
	}
	if !strings.HasPrefix(document.OpenAPI, "3.1.") {
		return nil, fmt.Errorf("OpenAPI version must be 3.1.x, got %q", document.OpenAPI)
	}
	if document.JSONSchemaDialect == "" {
		return nil, errors.New("jsonSchemaDialect is required for the OpenAPI 3.1 contract")
	}
	if err := document.Validate(ctx, openapi3.EnableMultiError()); err != nil {
		return nil, fmt.Errorf("validate OpenAPI document: %w", err)
	}

	declaredTags := make(map[string]struct{}, len(document.Tags))
	for _, tag := range document.Tags {
		if tag == nil || strings.TrimSpace(tag.Name) == "" {
			return nil, errors.New("every top-level tag must have a name")
		}
		declaredTags[tag.Name] = struct{}{}
	}

	contract := &Contract{Document: document, Operations: make(map[Route]*openapi3.Operation)}
	operationIDs := make(map[string]Route)
	paths := document.Paths.Keys()
	sort.Strings(paths)
	for _, path := range paths {
		pathItem := document.Paths.Value(path)
		if pathItem == nil {
			continue
		}
		methods := make([]string, 0, len(pathItem.Operations()))
		for method := range pathItem.Operations() {
			methods = append(methods, method)
		}
		sort.Strings(methods)
		for _, method := range methods {
			operation := pathItem.Operations()[method]
			if operation == nil {
				return nil, fmt.Errorf("%s %s has a null operation", method, path)
			}
			route := Route{Method: strings.ToUpper(method), Path: path}
			if _, exists := contract.Operations[route]; exists {
				return nil, fmt.Errorf("duplicate operation %s", route)
			}
			if !operationIDPattern.MatchString(operation.OperationID) {
				return nil, fmt.Errorf("%s operationId %q is missing or not lower camel case", route, operation.OperationID)
			}
			if previous, exists := operationIDs[operation.OperationID]; exists {
				return nil, fmt.Errorf("duplicate operationId %q on %s and %s", operation.OperationID, previous, route)
			}
			operationIDs[operation.OperationID] = route
			if len(operation.Tags) != 1 {
				return nil, fmt.Errorf("%s must have exactly one domain tag", route)
			}
			if _, exists := declaredTags[operation.Tags[0]]; !exists {
				return nil, fmt.Errorf("%s uses undeclared tag %q", route, operation.Tags[0])
			}
			if operation.Security == nil {
				return nil, fmt.Errorf("%s must explicitly declare its security boundary", route)
			}
			if required, ok := operation.Extensions["x-required-production-route"].(bool); !ok || !required {
				return nil, fmt.Errorf("%s is not marked as a required production route", route)
			}
			if strings.TrimSpace(operation.Summary) == "" || strings.TrimSpace(operation.Description) == "" {
				return nil, fmt.Errorf("%s must have a summary and tenant-aware description", route)
			}
			if operation.Responses == nil || operation.Responses.Len() == 0 {
				return nil, fmt.Errorf("%s must declare responses", route)
			}
			if err := validateSuccessResponse(route, operation.Responses); err != nil {
				return nil, err
			}
			if len(*operation.Security) > 0 {
				for _, status := range []string{"401", "403", "429", "500"} {
					if operation.Responses.Value(status) == nil {
						return nil, fmt.Errorf("%s must document %s error responses", route, status)
					}
				}
			}
			contract.Operations[route] = operation
		}
	}
	if len(contract.Operations) == 0 {
		return nil, errors.New("OpenAPI contract has no operations")
	}
	return contract, nil
}

func validateSuccessResponse(route Route, responses *openapi3.Responses) error {
	for _, status := range responses.Keys() {
		if !strings.HasPrefix(status, "2") {
			continue
		}
		response := responses.Value(status)
		if response == nil || response.Value == nil {
			return fmt.Errorf("%s has an unresolved %s response", route, status)
		}
		if status == "204" {
			return nil
		}
		mediaType := response.Value.Content.Get("application/json")
		if mediaType == nil || mediaType.Schema == nil {
			return fmt.Errorf("%s %s response must have an application/json schema", route, status)
		}
		return nil
	}
	return fmt.Errorf("%s has no successful response", route)
}

// SortedRoutes returns a deterministic operation inventory.
func (c *Contract) SortedRoutes() []Route {
	routes := make([]Route, 0, len(c.Operations))
	for route := range c.Operations {
		routes = append(routes, route)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes
}

// IsHTTPMethod reports whether method can appear as an OpenAPI operation.
func IsHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
