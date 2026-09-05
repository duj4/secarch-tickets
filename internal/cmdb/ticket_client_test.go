package cmdb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestSecArchTicketJQL(t *testing.T) {
	full := SecArchTicketJQL(false)
	if !strings.Contains(full, secArchRequestTypeReview) || !strings.Contains(full, secArchRequestTypeException) {
		t.Fatalf("full JQL does not contain both request types: %s", full)
	}
	if strings.Contains(full, "status != Closed") {
		t.Fatalf("full JQL unexpectedly excludes Closed tickets: %s", full)
	}
	if open := SecArchTicketJQL(true); !strings.HasSuffix(open, " AND status != Closed") {
		t.Fatalf("Open JQL does not exclude Closed tickets: %s", open)
	}
}

func TestListTicketsPaginatesAndNormalizes(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		if query.Get("jqlQuery") != "test-jql" {
			t.Errorf("jqlQuery = %q", query.Get("jqlQuery"))
		}
		if query.Get("pageSize") != "2" || query.Get("useTicketID") != "false" {
			t.Errorf("unexpected pagination/control query: %s", r.URL.RawQuery)
		}
		if len(query["fields"]) != 6 {
			t.Errorf("fields count = %d, want 6", len(query["fields"]))
		}

		startAt, _ := strconv.Atoi(query.Get("startAt"))
		response := TicketQueueAPIResponse{Total: 3, Issues: map[string]TicketQueueAPIResponseIssue{}}
		switch startAt {
		case 0:
			response.Issues["ITSM-20"] = TicketQueueAPIResponseIssue{
				Created: "2026-09-05T10:00:00.000+0800", Reporter: "reporter-b",
				Summary: "second", CMDBSystemName: []string{"System B (IACV-20)"},
			}
			response.Issues["ITSM-10"] = TicketQueueAPIResponseIssue{
				Created: "2026-09-04T10:00:00+08:00", Reporter: "reporter-a",
				Summary: "first", CMDBSystemName: []string{"System A (IACV-10)"},
			}
		case 2:
			resolved := "2026-09-06T09:00:00.000+0800"
			response.Issues["ITSM-30"] = TicketQueueAPIResponseIssue{
				Created: "2026-09-03T10:00:00.000+0800", Resolved: &resolved,
				Reporter: "reporter-c", Summary: "third", CMDBSystemName: []string{"IACV-30"},
			}
		default:
			t.Errorf("unexpected startAt %d", startAt)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		cfg:        Config{TicketAPIURL: server.URL, PageSize: 2},
	}
	tickets, err := client.ListTickets(t.Context(), "test-jql")
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if len(tickets) != 3 {
		t.Fatalf("tickets = %d, want 3", len(tickets))
	}
	if tickets[0].TicketNumber != "ITSM-10" || tickets[0].CMDBSystemKey != "IACV-10" {
		t.Fatalf("first ticket = %#v", tickets[0])
	}
	if tickets[2].TicketClosedAt == nil || tickets[2].CMDBSystemKey != "IACV-30" {
		t.Fatalf("third ticket = %#v", tickets[2])
	}
}

func TestTicketSystemReferenceRejectsMultipleSystems(t *testing.T) {
	_, err := ticketSystemReference([]string{"System A (IACV-1)", "System B (IACV-2)"})
	if err == nil {
		t.Fatal("expected multiple Systems to be rejected")
	}
}
