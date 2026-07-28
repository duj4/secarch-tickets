package cmdb

import (
	"time"
)

// TicketQueueAPIResponse represents the raw response from the CMDB ticket queue API.
type TicketQueueAPIResponse struct {
	Issues map[string]TicketQueueAPIResponseIssue `json:"issues"`
}

// TicketQueueAPIResponseIssue represents one ticket object returned by the CMDB ticket queue API.
//
// Note:
//   - TicketNumber is the external ticket number, such as ITSM-xxxxx.
type TicketQueueAPIResponseIssue struct {
	Assignee       *string  `json:"Assignee"`
	CMDBSystemName []string `json:"CMDB System Name"`
	Created        string   `json:"Created"`
	Reporter       string   `json:"Reporter"`
	Resolved       *string  `json:"Resolved"`
	Summary        string   `json:"Summary"`
}

// Ticket represents normalized ticket data used internally.
//
// Fields are cleaned and converted from the raw CMDB ticket queue response.
type Ticket struct {
	Assignee        *string
	CMDBSystemName  []string
	TicketCreatedAt time.Time
	Reporter        string
	TicketClosedAt  *time.Time
	Summary         string
	TicketNumber    string
}
