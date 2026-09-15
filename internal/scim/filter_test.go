package scim

import (
	"errors"
	"strings"
	"testing"
)

func TestParseFilterSupportedSubset(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		resource ResourceKind
		want     map[string]string
	}{
		{name: "user name", input: `userName eq "ada@example.test"`, resource: ResourceUsers, want: map[string]string{"username": "ada@example.test"}},
		{name: "case insensitive grammar", input: `USERNAME EQ "Ada@Example.Test" AnD externalId eq "HR-42"`, resource: ResourceUsers, want: map[string]string{"username": "Ada@Example.Test", "externalid": "HR-42"}},
		{name: "group", input: `displayName eq "Risk Reviewers"`, resource: ResourceGroups, want: map[string]string{"displayname": "Risk Reviewers"}},
		{name: "escaped string", input: `externalId eq "HR-\u0034\u0032"`, resource: ResourceUsers, want: map[string]string{"externalid": "HR-42"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filter, err := ParseFilter(test.input, test.resource)
			if err != nil {
				t.Fatal(err)
			}
			if len(filter.Clauses) != len(test.want) {
				t.Fatalf("clauses=%#v", filter.Clauses)
			}
			for attribute, expected := range test.want {
				if actual, ok := filter.Value(attribute); !ok || actual != expected {
					t.Fatalf("Value(%q)=(%q,%v), want %q", attribute, actual, ok, expected)
				}
			}
		})
	}
}

func TestParseFilterRejectsUnsupportedOrAmbiguousExpressions(t *testing.T) {
	tests := []struct {
		input    string
		resource ResourceKind
		want     error
	}{
		{`userName co "ada"`, ResourceUsers, ErrUnsupportedFilter},
		{`userName eq "ada" or externalId eq "42"`, ResourceUsers, ErrUnsupportedFilter},
		{`displayName eq "Risk" and externalId eq "42"`, ResourceGroups, ErrUnsupportedFilter},
		{`userName eq "ada" and userName eq "other"`, ResourceUsers, ErrInvalidFilter},
		{`userName eq ada`, ResourceUsers, ErrInvalidFilter},
		{`userName eq "unterminated`, ResourceUsers, ErrInvalidFilter},
		{strings.Repeat("a", maximumFilterBytes+1), ResourceUsers, ErrInvalidFilter},
	}
	for _, test := range tests {
		if _, err := ParseFilter(test.input, test.resource); !errors.Is(err, test.want) {
			t.Errorf("ParseFilter(%q) error=%v, want %v", test.input, err, test.want)
		}
	}
}
