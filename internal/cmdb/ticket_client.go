package cmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// GetTicket fetches and normalizes one SecArch ticket from the CMDB ticket API.
func (c *Client) GetTicket(ctx context.Context, ticketNumber string) (*Ticket, error) {
	resp, err := c.fetchTicket(ctx, ticketNumber)
	if err != nil {
		return nil, err
	}

	if len(resp.Issues) == 0 {
		return nil, fmt.Errorf("ticket not found: %s", ticketNumber)
	}

	if len(resp.Issues) > 1 {
		return nil, fmt.Errorf("unexpected multiple issues for %s", ticketNumber)
	}

	var (
		issue    TicketQueueAPIResponseIssue
		issueKey string
	)

	for k, v := range resp.Issues {
		issueKey = k
		issue = v
		break
	}

	var assigneePtr *string
	if issue.Assignee != nil {
		assigneePtr = issue.Assignee
	}

	const cmdbTimeLayout = "2006-01-02T15:04:05.000-0700"
	createdAt, err := time.Parse(cmdbTimeLayout, issue.Created)
	if err != nil {
		return nil, fmt.Errorf("parse created time(%s): %w", issue.Created, err)
	}

	var closedAt *time.Time
	if issue.Resolved != nil {
		t, err := time.Parse(cmdbTimeLayout, *issue.Resolved)
		if err != nil {
			return nil, err
		}
		closedAt = &t
	}

	return &Ticket{
		TicketNumber:    issueKey,
		Summary:         issue.Summary,
		Reporter:        issue.Reporter,
		Assignee:        assigneePtr,
		CMDBSystemName:  issue.CMDBSystemName,
		TicketCreatedAt: createdAt,
		TicketClosedAt:  closedAt,
	}, nil
}

func (c *Client) fetchTicket(ctx context.Context, ticketNumber string) (*TicketQueueAPIResponse, error) {
	requestURL, err := c.buildTicketQueueRequestURL(ticketNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to build ticket number request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ticket number request: %w", err)
	}

	req.Header.Set("accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query cmdb ticket API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cmdb ticket API returned status %d: %s", resp.StatusCode, string(body))
	}

	var out TicketQueueAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode cmdb ticket response: %w", err)
	}

	return &out, nil
}

// buildTicketQueueRequestURL builds the CMDB ticket API URL for one ticket number.
func (c *Client) buildTicketQueueRequestURL(ticketNumber string) (string, error) {
	baseURL, err := url.Parse(c.cfg.TicketAPIURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse cmdb ticket_api_url %q: %w", c.cfg.TicketAPIURL, err)
	}

	params := url.Values{}
	params.Add("fields", "Assignee")
	params.Add("fields", "CMDB System Name")
	params.Add("fields", "Created")
	params.Add("fields", "Reporter")
	params.Add("fields", "Resolved")
	params.Add("fields", "Summary")
	params.Set("issueKey", ticketNumber)
	params.Set("useTicketID", "false")

	baseURL.RawQuery = params.Encode()

	return baseURL.String(), nil
}
