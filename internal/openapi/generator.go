package openapi

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var pathParameterPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)

const (
	maximumOpenAPIBaseBytes  = 8 << 20
	maximumRouteCatalogBytes = 2 << 20
)

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
// Filenames are trusted operator-selected cmd/openapi CLI configuration, not
// request input. Do not expose this filesystem-based generator through HTTP.
func Generate(baseFilename, routesFilename string) ([]byte, error) {
	baseFile, err := os.Open(baseFilename) // #nosec G304 -- cmd/openapi --base is a trusted operator-selected local contract path; no HTTP handler supplies it.
	if err != nil {
		return nil, fmt.Errorf("read OpenAPI base: %w", err)
	}
	baseData, readErr := io.ReadAll(io.LimitReader(baseFile, maximumOpenAPIBaseBytes+1))
	closeErr := baseFile.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(wrapError("read OpenAPI base", readErr), wrapError("close OpenAPI base", closeErr))
	}
	if len(baseData) > maximumOpenAPIBaseBytes {
		return nil, errors.New("OpenAPI base exceeds operator contract size limit")
	}
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(baseData))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode OpenAPI base: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("OpenAPI base must contain exactly one JSON document")
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
	file, err := os.Open(filename) // #nosec G304 -- Only cmd/openapi --routes and deterministic repository tests select this local catalog path; no HTTP-controlled path.
	if err != nil {
		return nil, fmt.Errorf("open route catalog: %w", err)
	}

	limited := &io.LimitedReader{R: file, N: maximumRouteCatalogBytes + 1}
	reader := csv.NewReader(limited)
	reader.Comment = '#'
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = 10
	records, readErr := reader.ReadAll()
	closeErr := file.Close()
	if limited.N == 0 {
		return nil, errors.New("route catalog exceeds operator contract size limit")
	}
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(
			wrapError("read route catalog", readErr),
			wrapError("close route catalog", closeErr),
		)
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
		if !operationIDPattern.MatchString(route.OperationID) {
			return nil, fmt.Errorf("route catalog record %d has invalid operationId %q", line, route.OperationID)
		}
		switch route.Security {
		case "public", "bearer", "apiKey", "scim":
		default:
			return nil, fmt.Errorf("route catalog record %d has invalid security %q", line, route.Security)
		}
		route.Queries, err = parseQueries(record[9])
		if err != nil {
			return nil, fmt.Errorf("route catalog record %d: %w", line, err)
		}
		for flag := range route.Flags {
			switch flag {
			case "paginated", "deprecated", "optional-body", "feature-gated", "quota-gated", "created-or-ok", "csv-body", "idempotent", "change-reason", "step-up", "identity-security", "multipart-body", "multipart-only", "binary-response", "temporary-redirect", "payload-limited":
			default:
				return nil, fmt.Errorf("route catalog record %d has unknown flag %q", line, flag)
			}
		}
		if route.Flags["multipart-body"] && route.Flags["multipart-only"] {
			return nil, fmt.Errorf("route catalog record %d cannot combine multipart-body and multipart-only", line)
		}
		if (route.Flags["multipart-body"] || route.Flags["multipart-only"]) && route.RequestSchema == "" {
			return nil, fmt.Errorf("route catalog record %d uses multipart transport without a request schema", line)
		}
		if route.Tag == "automation" && route.Method != http.MethodGet {
			return nil, fmt.Errorf("route catalog record %d makes the read-only automation API mutable", line)
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

func wrapError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
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
	if route.Security == "scim" {
		operation["x-scim-protocol"] = true
	}
	parameters := parametersFor(route)
	if len(parameters) > 0 {
		operation["parameters"] = parameters
	}
	if route.RequestSchema != "" {
		mediaType := "application/json"
		if route.Security == "scim" {
			mediaType = "application/scim+json"
		}
		if route.Flags["csv-body"] {
			mediaType = "text/csv"
		}
		content := map[string]any{mediaType: map[string]any{
			"schema": schemaRef(route.RequestSchema),
		}}
		if route.Flags["multipart-only"] {
			content = map[string]any{"multipart/form-data": map[string]any{
				"schema": schemaRef(route.RequestSchema),
			}}
		} else if route.Flags["multipart-body"] {
			content["multipart/form-data"] = map[string]any{
				"schema": schemaRef(route.RequestSchema + "Multipart"),
			}
		}
		operation["requestBody"] = map[string]any{
			"required": !route.Flags["optional-body"],
			"content":  content,
		}
	}
	if route.Flags["binary-response"] {
		operation["x-binary-response"] = true
	}
	if route.Method == http.MethodPost && route.Path == "/api/v1/settings/diagnostics/support-bundle" {
		responses := operation["responses"].(map[string]any)
		success := responses["200"].(map[string]any)
		headers := success["headers"].(map[string]any)
		headers["X-Support-Bundle-SHA256"] = map[string]any{
			"description": "Lowercase SHA-256 of the exact downloaded ZIP bytes; clients must verify before opening.",
			"schema":      map[string]any{"type": "string", "pattern": `^[0-9a-f]{64}$`},
		}
		headers["Content-Length"].(map[string]any)["schema"].(map[string]any)["maximum"] = 65536
		responses["503"] = responseRef("DependencyUnavailable")
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
			schema["minimum"] = 1
		}
		lowerName := strings.ToLower(query.Name)
		if route.Security == "scim" && lowerName == "count" {
			schema["minimum"] = 0
			schema["maximum"] = 200
		}
		if route.Security == "scim" && lowerName == "startindex" {
			schema["default"] = 1
		}
		if route.Security == "scim" && lowerName == "filter" {
			schema["maxLength"] = 1024
		}
		if query.Type == "string" && (lowerName == "id" || strings.HasSuffix(lowerName, "_id")) {
			schema["format"] = "uuid"
		}
		if query.Type == "string" && strings.HasSuffix(lowerName, "_before") {
			schema["format"] = "date"
		}
		parameters = append(parameters, map[string]any{
			"name": query.Name, "in": "query", "required": query.Required,
			"description": "Operation-specific " + query.Name + " value.", "schema": schema,
		})
	}
	if route.Flags["idempotent"] {
		parameters = append(parameters, map[string]any{"$ref": "#/components/parameters/IdempotencyKey"})
	}
	if route.Flags["change-reason"] {
		parameters = append(parameters, map[string]any{"$ref": "#/components/parameters/ChangeReason"})
	}
	if route.Flags["step-up"] {
		parameters = append(parameters, map[string]any{"$ref": "#/components/parameters/StepUpToken"})
	}
	if route.Security == "scim" {
		switch route.Method {
		case http.MethodPut, http.MethodPatch, http.MethodDelete:
			parameters = append(parameters, map[string]any{
				"name": "If-Match", "in": "header", "required": true,
				"description": "Current weak SCIM resource ETag used for optimistic concurrency.",
				"schema":      map[string]any{"type": "string", "pattern": `^W/\"[1-9][0-9]*\"$`},
			})
		case http.MethodGet:
			if pathParameterPattern.MatchString(route.Path) {
				parameters = append(parameters, map[string]any{
					"name": "If-None-Match", "in": "header", "required": false,
					"description": "Return 304 when this exact SCIM ETag is still current.",
					"schema":      map[string]any{"type": "string"},
				})
			}
		}
	}
	return parameters
}

func responsesFor(route catalogRoute) map[string]any {
	status := strconv.Itoa(route.Status)
	success := map[string]any{"description": httpSuccessDescription(route.Status)}
	if route.Flags["deprecated"] {
		success["headers"] = deprecatedResponseHeaders()
	}
	if route.Status != 204 && route.Flags["binary-response"] {
		success["headers"] = binaryResponseHeaders()
		success["content"] = map[string]any{"application/octet-stream": map[string]any{
			"schema": map[string]any{"type": "string", "format": "binary"},
		}}
	} else if route.Status != 204 {
		mediaType := "application/json"
		if route.Security == "scim" {
			mediaType = "application/scim+json"
			success["headers"] = map[string]any{
				"ETag":     map[string]any{"description": "Weak resource version ETag.", "schema": map[string]any{"type": "string"}},
				"Location": map[string]any{"description": "Canonical SCIM resource location on creation.", "schema": map[string]any{"type": "string"}},
			}
		}
		success["content"] = map[string]any{mediaType: map[string]any{
			"schema": schemaRef(defaultSchema(route.ResponseSchema)),
		}}
	}
	responses := map[string]any{status: success}
	if route.Flags["temporary-redirect"] {
		responses["307"] = map[string]any{
			"description": "A short-lived, private object-store download URL was created.",
			"headers": map[string]any{
				"Location":        map[string]any{"description": "Short-lived signed download URL.", "schema": map[string]any{"type": "string", "format": "uri"}},
				"Cache-Control":   map[string]any{"description": "Prevents redirect caching.", "schema": map[string]any{"type": "string"}},
				"Referrer-Policy": map[string]any{"description": "Prevents disclosure of the signed URL through referrers.", "schema": map[string]any{"type": "string"}},
			},
		}
	}
	if route.Flags["created-or-ok"] {
		created := map[string]any{
			"description": "Resource created.",
			"content": map[string]any{"application/json": map[string]any{
				"schema": schemaRef(defaultSchema(route.ResponseSchema)),
			}},
		}
		if route.Flags["deprecated"] {
			created["headers"] = deprecatedResponseHeaders()
		}
		responses["201"] = created
	}
	errorResponse := "BadRequest"
	if route.Security == "scim" {
		errorResponse = "SCIMErrorResponse"
	}
	if route.Security == "scim" || route.RequestSchema != "" || route.Method == "POST" || route.Method == "PUT" || route.Method == "PATCH" {
		responses["400"] = responseRef(errorResponse)
	}
	if route.Security != "public" {
		unauthorizedResponse := "Unauthorized"
		forbiddenResponse := "Forbidden"
		rateLimitResponse := "RateLimited"
		if route.Security == "scim" {
			unauthorizedResponse, forbiddenResponse, rateLimitResponse = "SCIMErrorResponse", "SCIMErrorResponse", "SCIMErrorResponse"
		}
		responses["401"] = responseRef(unauthorizedResponse)
		if route.Flags["feature-gated"] {
			if route.Security == "scim" {
				responses["402"] = responseRef("SCIMErrorResponse")
				responses["403"] = responseRef("SCIMErrorResponse")
				responses["503"] = responseRef("SCIMErrorResponse")
			} else {
				responses["402"] = responseRef("PaymentRequired")
				responses["403"] = responseRef("FeatureForbidden")
				responses["503"] = responseRef("EvaluationUnavailable")
			}
		} else {
			responses["403"] = responseRef(forbiddenResponse)
		}
		responses["429"] = responseRef(rateLimitResponse)
	}
	if route.Flags["quota-gated"] {
		responses["402"] = responseRef("PaymentRequired")
		responses["503"] = responseRef("EvaluationUnavailable")
	}
	if route.Flags["identity-security"] {
		responses["401"] = responseRef("Unauthorized")
		responses["409"] = responseRef("Conflict")
		responses["429"] = responseRef("RateLimited")
	}
	if route.Flags["payload-limited"] {
		responses["413"] = responseRef("PayloadTooLarge")
	}
	if route.Path != "/health" && route.Path != "/health/live" && route.Path != "/health/ready" {
		if responses["503"] == nil {
			if route.Security == "scim" {
				responses["503"] = responseRef("SCIMErrorResponse")
			} else {
				responses["503"] = responseRef("DependencyUnavailable")
			}
		}
	}
	if pathParameterPattern.MatchString(route.Path) {
		if route.Security == "scim" {
			responses["404"] = responseRef("SCIMErrorResponse")
		} else {
			responses["404"] = responseRef("NotFound")
		}
	}
	if route.Method != "GET" && route.Method != "HEAD" && route.Status != 204 {
		if route.Security == "scim" {
			responses["409"] = responseRef("SCIMErrorResponse")
		} else {
			responses["409"] = responseRef("Conflict")
			responses["422"] = responseRef("UnprocessableEntity")
		}
	}
	if route.Security == "scim" {
		responses["415"] = responseRef("SCIMErrorResponse")
		responses["500"] = responseRef("SCIMErrorResponse")
		if route.Method == http.MethodPut || route.Method == http.MethodPatch || route.Method == http.MethodDelete {
			responses["412"] = responseRef("SCIMErrorResponse")
			responses["428"] = responseRef("SCIMErrorResponse")
		}
		if route.Method == http.MethodGet && pathParameterPattern.MatchString(route.Path) {
			responses["304"] = map[string]any{"description": "The exact requested SCIM resource version is unchanged."}
		}
	} else {
		responses["500"] = responseRef("InternalError")
	}
	if route.Path == "/health/ready" {
		responses["503"] = responseRef("ServiceUnavailable")
	}
	return responses
}

func binaryResponseHeaders() map[string]any {
	return map[string]any{
		"Content-Disposition":     map[string]any{"description": "Safe attachment filename.", "schema": map[string]any{"type": "string"}},
		"Content-Length":          map[string]any{"description": "Object size in bytes.", "schema": map[string]any{"type": "integer", "minimum": 0}},
		"Cache-Control":           map[string]any{"description": "Private no-store cache policy.", "schema": map[string]any{"type": "string"}},
		"X-Content-Type-Options":  map[string]any{"description": "Disables MIME sniffing.", "schema": map[string]any{"type": "string"}},
		"Content-Security-Policy": map[string]any{"description": "Restrictive document sandbox.", "schema": map[string]any{"type": "string"}},
	}
}

func deprecatedResponseHeaders() map[string]any {
	return map[string]any{
		"Deprecation": map[string]any{"$ref": "#/components/headers/Deprecation"},
		"Sunset":      map[string]any{"$ref": "#/components/headers/Sunset"},
		"Link":        map[string]any{"$ref": "#/components/headers/SuccessorLink"},
	}
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
	case "scim":
		return []any{map[string]any{"scimBearerAuth": []any{}}}
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
	case "scim":
		return "scim_token_organization_id"
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
	case "scim":
		return base + " The tenant, IdP actor, exact resource scope, and per-token rate limit are derived only from the hashed SCIM bearer credential; tenant headers and browser JWTs are not accepted."
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
	seen := make(map[string]struct{})
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
		if _, duplicate := seen[parts[0]]; duplicate {
			return nil, fmt.Errorf("duplicate query parameter %q", parts[0])
		}
		seen[parts[0]] = struct{}{}
		queries = append(queries, catalogQuery{Name: parts[0], Type: typeName, Required: required})
	}
	return queries, nil
}
