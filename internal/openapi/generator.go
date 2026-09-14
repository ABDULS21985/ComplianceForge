package openapi

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var pathParameterPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)

type catalogRoute struct {
	Method         string
	Path           string
	OperationID    string
	Tag            string
	Security       string
	RequestSchema  string
	ResponseSchema string
	Status         int
	Flags          map[string]bool
	Queries        []catalogQuery
}

type catalogQuery struct {
	Name     string
	Type     string
	Required bool
}

// Generate produces the deterministic single-file OpenAPI artifact from the
// human-reviewable base document and production-route catalog.
func Generate(baseFilename, routesFilename string) ([]byte, error) {
	baseData, err := os.ReadFile(baseFilename)
	if err != nil {
		return nil, fmt.Errorf("read OpenAPI base: %w", err)
	}
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(baseData))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode OpenAPI base: %w", err)
	}
	if _, exists := document["paths"]; exists {
		return nil, errors.New("OpenAPI base must not contain paths; routes.csv is authoritative")
	}

	routes, err := readRouteCatalog(routesFilename)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]any)
	for _, route := range routes {
		pathItem, _ := paths[route.Path].(map[string]any)
		if pathItem == nil {
			pathItem = make(map[string]any)
			paths[route.Path] = pathItem
		}
		method := strings.ToLower(route.Method)
		if _, duplicate := pathItem[method]; duplicate {
			return nil, fmt.Errorf("duplicate route %s %s", route.Method, route.Path)
		}
		pathItem[method] = operationFor(route)
	}
	document["paths"] = paths

	output, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OpenAPI document: %w", err)
	}
	return append(output, '\n'), nil
}

func readRouteCatalog(filename string) ([]catalogRoute, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open route catalog: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comment = '#'
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = 10
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read route catalog: %w", err)
	}
	routes := make([]catalogRoute, 0, len(records))
	for index, record := range records {
		line := index + 1
		method := strings.ToUpper(strings.TrimSpace(record[0]))
		if !IsHTTPMethod(method) {
			return nil, fmt.Errorf("route catalog record %d has unsupported method %q", line, record[0])
		}
		status, err := strconv.Atoi(strings.TrimSpace(record[7]))
		if err != nil || status < 200 || status > 299 {
			return nil, fmt.Errorf("route catalog record %d has invalid success status %q", line, record[7])
		}
		route := catalogRoute{
			Method: method, Path: strings.TrimSpace(record[1]), OperationID: strings.TrimSpace(record[2]),
			Tag: strings.TrimSpace(record[3]), Security: strings.TrimSpace(record[4]),
			RequestSchema: strings.TrimSpace(record[5]), ResponseSchema: strings.TrimSpace(record[6]),
			Status: status, Flags: parseFlags(record[8]),
		}
		if route.Path == "" || !strings.HasPrefix(route.Path, "/") || route.OperationID == "" || route.Tag == "" {
			return nil, fmt.Errorf("route catalog record %d is missing path, operationId, or tag", line)
		}
		switch route.Security {
		case "public", "bearer", "apiKey":
		default:
			return nil, fmt.Errorf("route catalog record %d has invalid security %q", line, route.Security)
		}
		route.Queries, err = parseQueries(record[9])
		if err != nil {
			return nil, fmt.Errorf("route catalog record %d: %w", line, err)
		}
		routes = append(routes, route)
	}
	if len(routes) == 0 {
		return nil, errors.New("route catalog is empty")
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes, nil
}

func operationFor(route catalogRoute) map[string]any {
	operation := map[string]any{
		"operationId":                 route.OperationID,
		"summary":                     humanizeOperationID(route.OperationID),
		"description":                 tenantDescription(route),
		"tags":                        []any{route.Tag},
		"security":                    securityRequirement(route.Security),
		"responses":                   responsesFor(route),
		"x-required-production-route": true,
		"x-tenant-context":            tenantContext(route.Security),
	}
	parameters := parametersFor(route)
	if len(parameters) > 0 {
		operation["parameters"] = parameters
	}
	if route.RequestSchema != "" {
		operation["requestBody"] = map[string]any{
			"required": !route.Flags["optional-body"],
			"content": map[string]any{"application/json": map[string]any{
				"schema": schemaRef(route.RequestSchema),
			}},
		}
	}
	if route.Flags["deprecated"] {
		operation["deprecated"] = true
		operation["description"] = operation["description"].(string) + " This compatibility alias is deprecated; clients must migrate to the canonical operation."
	}
	return operation
}

func parametersFor(route catalogRoute) []any {
	parameters := make([]any, 0)
	for _, match := range pathParameterPattern.FindAllStringSubmatch(route.Path, -1) {
		name := match[1]
		schema := map[string]any{"type": "string"}
		lowerName := strings.ToLower(name)
		if lowerName == "id" || strings.HasSuffix(lowerName, "id") {
			schema["format"] = "uuid"
		}
		parameters = append(parameters, map[string]any{
			"name": name, "in": "path", "required": true,
			"description": "Canonical " + name + " path identifier.", "schema": schema,
		})
	}
	if route.Flags["paginated"] {
		parameters = append(parameters,
			map[string]any{"$ref": "#/components/parameters/Page"},
			map[string]any{"$ref": "#/components/parameters/PageSize"},
		)
	}
	for _, query := range route.Queries {
		schema := map[string]any{"type": query.Type}
		if query.Type == "integer" {
			schema["minimum"] = 0
		}
		parameters = append(parameters, map[string]any{
			"name": query.Name, "in": "query", "required": query.Required,
			"description": "Operation-specific " + query.Name + " value.", "schema": schema,
		})
	}
	return parameters
}

func responsesFor(route catalogRoute) map[string]any {
	status := strconv.Itoa(route.Status)
	success := map[string]any{"description": httpSuccessDescription(route.Status)}
	if route.Status != 204 {
		success["content"] = map[string]any{"application/json": map[string]any{
			"schema": schemaRef(defaultSchema(route.ResponseSchema)),
		}}
	}
	responses := map[string]any{status: success}
	if route.RequestSchema != "" || route.Method == "POST" || route.Method == "PUT" || route.Method == "PATCH" {
		responses["400"] = responseRef("BadRequest")
	}
	if route.Security != "public" {
		responses["401"] = responseRef("Unauthorized")
		responses["403"] = responseRef("Forbidden")
		responses["429"] = responseRef("RateLimited")
	}
	if pathParameterPattern.MatchString(route.Path) {
		responses["404"] = responseRef("NotFound")
	}
	if route.Method != "GET" && route.Method != "HEAD" && route.Status != 204 {
		responses["409"] = responseRef("Conflict")
	}
	responses["500"] = responseRef("InternalError")
	if route.Path == "/health/ready" {
		responses["503"] = responseRef("ServiceUnavailable")
	}
	return responses
}

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func responseRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/responses/" + name}
}

func defaultSchema(name string) string {
	if name == "" {
		return "GenericResponse"
	}
	return name
}

func securityRequirement(security string) []any {
	switch security {
	case "public":
		return []any{}
	case "apiKey":
		return []any{map[string]any{"apiKeyAuth": []any{}}}
	default:
		return []any{map[string]any{"bearerAuth": []any{}}}
	}
}

func tenantContext(security string) string {
	switch security {
	case "apiKey":
		return "api_key_principal"
	case "bearer":
		return "jwt_organization_id_claim"
	default:
		return "none"
	}
}

func tenantDescription(route catalogRoute) string {
	base := humanizeOperationID(route.OperationID) + "."
	switch route.Security {
	case "bearer":
		return base + " The organization boundary is derived from the validated access-token claim; callers cannot select another tenant by header or body field."
	case "apiKey":
		return base + " Read-only automation operation. The organization and exact read scope are derived from the API-key principal."
	default:
		return base + " This operation does not accept caller-selected tenant context."
	}
}

func humanizeOperationID(operationID string) string {
	var output strings.Builder
	for index, r := range operationID {
		if index > 0 && unicode.IsUpper(r) {
			output.WriteByte(' ')
		}
		if index == 0 {
			r = unicode.ToUpper(r)
		}
		output.WriteRune(r)
	}
	return output.String()
}

func httpSuccessDescription(status int) string {
	switch status {
	case 201:
		return "Resource created."
	case 202:
		return "Operation accepted for asynchronous processing."
	case 204:
		return "Operation completed; no response body."
	default:
		return "Operation completed successfully."
	}
}

func parseFlags(value string) map[string]bool {
	flags := make(map[string]bool)
	for _, flag := range strings.Split(value, ";") {
		flag = strings.TrimSpace(flag)
		if flag != "" {
			flags[flag] = true
		}
	}
	return flags
}

func parseQueries(value string) ([]catalogQuery, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	queries := make([]catalogQuery, 0)
	for _, encoded := range strings.Split(value, ";") {
		parts := strings.Split(strings.TrimSpace(encoded), ":")
		if len(parts) != 3 || parts[0] == "" {
			return nil, fmt.Errorf("invalid query descriptor %q", encoded)
		}
		typeName := parts[1]
		if typeName != "string" && typeName != "integer" && typeName != "boolean" {
			return nil, fmt.Errorf("invalid query type %q", typeName)
		}
		required, err := strconv.ParseBool(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid query required flag %q", parts[2])
		}
		queries = append(queries, catalogQuery{Name: parts[0], Type: typeName, Required: required})
	}
	return queries, nil
}
