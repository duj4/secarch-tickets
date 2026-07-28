package secarch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"secarch-tickets/internal/cmdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TicketService coordinates ticket-related business logic.
type TicketService struct {
	pool       *pgxpool.Pool
	cmdbClient *cmdb.Client
}

// NewTicketService creates a TicketService.
func NewTicketService(pool *pgxpool.Pool, cmdbClient *cmdb.Client) *TicketService {
	return &TicketService{
		pool:       pool,
		cmdbClient: cmdbClient,
	}
}

// CreateTicket fetches a ticket from CMDB, validates it, and stores it.
func (s *TicketService) CreateTicket(ctx context.Context, ticketNumber string, expectedDate time.Time) (string, error) {
	// Trim again at the service boundary for safety.
	ticketNumber = strings.TrimSpace(ticketNumber)
	if ticketNumber == "" {
		return "", fmt.Errorf("ticket_number cannot be empty")
	}

	// Fetch the latest ticket data from CMDB.
	ticket, err := s.cmdbClient.GetTicket(ctx, ticketNumber)
	if err != nil {
		return "", fmt.Errorf("failed to fetch ticket from from CMDB: %w", err)
	}

	// Validate the normalized CMDB response.
	if ticket == nil || strings.TrimSpace(ticket.TicketNumber) == "" {
		return "", fmt.Errorf("invalid ticket data from CMDB")
	}

	// Store the ticket in the database.
	status, err := UpsertTicket(ctx, s.pool, ticket, expectedDate)
	if err != nil {
		return "", fmt.Errorf("upsert ticket %s: %w", ticketNumber, err)
	}

	return status, nil
}

// ListTickets returns stored tickets after a best-effort CMDB refresh.
func (s *TicketService) ListTickets(ctx context.Context) ([]StoredTicket, error) {
	// Read the current ticket set from the database.
	tickets, err := ListTickets(ctx, s.pool)
	if err != nil {
		return nil, err
	}

	// Refresh from CMDB on a best-effort basis.
	RefreshTicketsFromCMDB(ctx, s.pool, s.cmdbClient, tickets)

	// Read again to return refreshed data.
	tickets, err = ListTickets(ctx, s.pool)
	if err != nil {
		return nil, err
	}

	return tickets, nil
}

// UpdateExpectedDate updates expected_date for a ticket.
func (s *TicketService) UpdateExpectedDate(ctx context.Context, ticketNumber string, expectedDate time.Time) error {
	err := UpdateExpectedDate(ctx, s.pool, ticketNumber, expectedDate)
	if err != nil {
		return err
	}

	return nil
}

// DeleteTicket deletes a ticket by ticket number.
func (s *TicketService) DeleteTicket(ctx context.Context, ticketNumber string) error {
	err := DeleteTicket(ctx, s.pool, ticketNumber)
	if err != nil {
		return err
	}

	return nil
}
