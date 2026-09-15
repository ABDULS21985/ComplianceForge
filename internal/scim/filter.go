package scim

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidFilter     = errors.New("invalid SCIM filter")
	ErrUnsupportedFilter = errors.New("unsupported SCIM filter")
)

type ResourceKind string

const (
	ResourceUsers      ResourceKind = "Users"
	ResourceGroups     ResourceKind = "Groups"
	maximumFilterBytes              = 1024
)

type Filter struct {
	Clauses []FilterClause
}

type FilterClause struct {
	Attribute string
	Value     string
}

func (f Filter) Value(attribute string) (string, bool) {
	attribute = strings.ToLower(strings.TrimSpace(attribute))
	for _, clause := range f.Clauses {
		if clause.Attribute == attribute {
			return clause.Value, true
		}
	}
	return "", false
}

// ParseFilter accepts the deliberately bounded, indexable SCIM subset used by
// the directory repository: equality clauses joined by AND. It rejects rather
// than silently weakening unsupported operators or attributes.
func ParseFilter(raw string, resource ResourceKind) (Filter, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Filter{}, nil
	}
	if len(raw) > maximumFilterBytes || !utf8.ValidString(raw) {
		return Filter{}, fmt.Errorf("%w: filter exceeds the supported size", ErrInvalidFilter)
	}
	parser := filterParser{input: raw}
	filter := Filter{Clauses: make([]FilterClause, 0, 2)}
	seen := make(map[string]struct{})
	for {
		attribute, err := parser.identifier()
		if err != nil {
			return Filter{}, err
		}
		attribute = strings.ToLower(attribute)
		if !allowedFilterAttribute(resource, attribute) {
			return Filter{}, fmt.Errorf("%w: attribute %q cannot be filtered", ErrUnsupportedFilter, attribute)
		}
		if _, duplicate := seen[attribute]; duplicate {
			return Filter{}, fmt.Errorf("%w: duplicate attribute %q", ErrInvalidFilter, attribute)
		}
		operator, err := parser.identifier()
		if err != nil {
			return Filter{}, err
		}
		if !strings.EqualFold(operator, "eq") {
			return Filter{}, fmt.Errorf("%w: operator %q is not supported", ErrUnsupportedFilter, operator)
		}
		value, err := parser.quotedString()
		if err != nil {
			return Filter{}, err
		}
		if value == "" || utf8.RuneCountInString(value) > 320 {
			return Filter{}, fmt.Errorf("%w: filter value is empty or too long", ErrInvalidFilter)
		}
		filter.Clauses = append(filter.Clauses, FilterClause{Attribute: attribute, Value: value})
		seen[attribute] = struct{}{}
		if len(filter.Clauses) > 2 {
			return Filter{}, fmt.Errorf("%w: too many filter clauses", ErrUnsupportedFilter)
		}
		parser.skipSpace()
		if parser.eof() {
			break
		}
		conjunction, err := parser.identifier()
		if err != nil {
			return Filter{}, err
		}
		if !strings.EqualFold(conjunction, "and") {
			return Filter{}, fmt.Errorf("%w: only AND conjunctions are supported", ErrUnsupportedFilter)
		}
	}
	return filter, nil
}

func allowedFilterAttribute(resource ResourceKind, attribute string) bool {
	switch resource {
	case ResourceUsers:
		return attribute == "username" || attribute == "externalid"
	case ResourceGroups:
		return attribute == "displayname"
	default:
		return false
	}
}

type filterParser struct {
	input string
	index int
}

func (p *filterParser) eof() bool { return p.index >= len(p.input) }

func (p *filterParser) skipSpace() {
	for !p.eof() {
		r, size := utf8.DecodeRuneInString(p.input[p.index:])
		if !unicode.IsSpace(r) {
			return
		}
		p.index += size
	}
}

func (p *filterParser) identifier() (string, error) {
	p.skipSpace()
	start := p.index
	for !p.eof() {
		r, size := utf8.DecodeRuneInString(p.input[p.index:])
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == ':' || r == '_' || r == '-') {
			break
		}
		p.index += size
	}
	if p.index == start {
		return "", fmt.Errorf("%w: expected an attribute, operator, or conjunction at byte %d", ErrInvalidFilter, p.index)
	}
	return p.input[start:p.index], nil
}

func (p *filterParser) quotedString() (string, error) {
	p.skipSpace()
	if p.eof() || p.input[p.index] != '"' {
		return "", fmt.Errorf("%w: filter values must be JSON-quoted strings", ErrInvalidFilter)
	}
	start := p.index
	p.index++
	escaped := false
	for !p.eof() {
		current := p.input[p.index]
		p.index++
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' {
			escaped = true
			continue
		}
		if current == '"' {
			encoded := p.input[start:p.index]
			value, err := strconv.Unquote(encoded)
			if err != nil || !utf8.ValidString(value) {
				return "", fmt.Errorf("%w: malformed quoted filter value", ErrInvalidFilter)
			}
			return value, nil
		}
	}
	return "", fmt.Errorf("%w: unterminated quoted filter value", ErrInvalidFilter)
}
