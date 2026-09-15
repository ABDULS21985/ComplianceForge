package service

import (
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/complianceforge/platform/internal/models"
)

func FuzzDirectoryCSVParser(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("email,first_name,last_name\nada@example.test,Ada,Lovelace\n"),
		[]byte("email,email\na@example.test,b@example.test\n"),
		[]byte("unknown\nvalue\n"),
		{0xff, 0xfe},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, content []byte) {
		if len(content) > maximumDirectoryImportBytes+1024 {
			t.Skip()
		}
		rows, digest, err := parseDirectoryCSV(content)
		if err != nil {
			return
		}
		if len(rows) == 0 || len(rows) > maximumDirectoryImportRows {
			t.Fatalf("accepted invalid row count %d", len(rows))
		}
		if len(digest) != 64 {
			t.Fatalf("digest length = %d", len(digest))
		}
		if _, err := hex.DecodeString(digest); err != nil {
			t.Fatalf("digest is not hexadecimal: %v", err)
		}
		for index, row := range rows {
			if row.RowNumber != index+2 || row.Email == "" || row.RoleSlug == "" {
				t.Fatalf("accepted malformed normalized row: %#v", row)
			}
		}
	})
}

func FuzzDynamicDirectoryGroupRule(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"departments":["Security"]}`),
		[]byte(`{"statuses":["active"]}`),
		[]byte(`{"unknown":true}`),
		[]byte(`null`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 17<<10 {
			t.Skip()
		}
		rule, err := validateDynamicGroupRule(json.RawMessage(raw))
		if err != nil {
			return
		}
		if len(rule.Departments)+len(rule.Locations)+len(rule.Statuses)+len(rule.RoleSlugs) == 0 {
			t.Fatal("empty dynamic rule was accepted")
		}
		if len(rule.Departments) > 50 || len(rule.Locations) > 50 || len(rule.Statuses) > 4 || len(rule.RoleSlugs) > 50 {
			t.Fatalf("oversized dynamic rule was accepted: %#v", rule)
		}
	})
}

func FuzzFindingAndIncidentStateMachines(f *testing.F) {
	f.Add("open", "in_progress")
	f.Add("closed", "open")
	f.Add("unknown", "unknown")
	f.Add("reported", "triaged")
	f.Fuzz(func(t *testing.T, from, to string) {
		findingFrom, findingTo := models.FindingStatus(from), models.FindingStatus(to)
		findingKnown := allowed(from, findingStatuses()...) && allowed(to, findingStatuses()...)
		if validFindingTransition(findingFrom, findingTo) && !findingKnown {
			t.Fatalf("finding transition accepted unknown state %q -> %q", from, to)
		}
		incidentFrom, incidentTo := models.IncidentStatus(from), models.IncidentStatus(to)
		if isValidIncidentTransition(incidentFrom, incidentTo) && (!validIncidentStatus(incidentFrom) || !validIncidentStatus(incidentTo)) {
			t.Fatalf("incident transition accepted unknown state %q -> %q", from, to)
		}
	})
}

func FuzzPaginationAndAuditDateParsers(f *testing.F) {
	f.Add(0, 0, "2026-09-14")
	f.Add(-1, 101, "not-a-date")
	f.Add(1, 100, "2026-09-14T23:59:59Z")
	f.Fuzz(func(t *testing.T, page, pageSize int, date string) {
		if len(date) > 4096 {
			t.Skip()
		}
		for name, pagination := range map[string]models.PaginationRequest{
			"audit":     normalizeAuditPagination(models.PaginationRequest{Page: page, PageSize: pageSize}),
			"incident":  normalizeIncidentPagination(models.PaginationRequest{Page: page, PageSize: pageSize}),
			"directory": normalizeDirectoryPagination(models.PaginationRequest{Page: page, PageSize: pageSize}),
		} {
			if pagination.Page < 1 || pagination.PageSize < 1 || pagination.PageSize > 100 {
				t.Fatalf("%s pagination escaped bounds: %#v", name, pagination)
			}
		}
		parsed, err := parseAuditDate(date)
		if err == nil && (parsed.Location() != nil && parsed.Location() != time.UTC || parsed.Hour() != 0 || parsed.Minute() != 0 || parsed.Second() != 0 || parsed.Nanosecond() != 0) {
			t.Fatalf("audit date was not normalized to UTC date: %v", parsed)
		}
	})
}
