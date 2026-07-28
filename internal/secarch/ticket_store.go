package secarch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"secarch-tickets/internal/cmdb"
	"secarch-tickets/internal/db"
	"secarch-tickets/internal/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StoredTicket represents a ticket row stored in PostgreSQL.
type StoredTicket struct {
	ID              int64      `json:"id"`
	TicketNumber    string     `json:"ticket_number"`
	Summary         string     `json:"summary"`
	Reporter        string     `json:"reporter"`
	Assignee        *string    `json:"assignee"`
	CMDBSystemName  []string   `json:"cmdb_system_name"`
	TicketCreatedAt time.Time  `json:"ticket_created_at"`
	TicketClosedAt  *time.Time `json:"ticket_closed_at"`
	ExpectedDate    time.Time  `json:"expected_date"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// UpsertTicket inserts or updates a ticket in secarch_tickets.tickets.
//
// Ticket data comes from CMDB, while expectedDate comes from user input.
func UpsertTicket(ctx context.Context, pool *pgxpool.Pool, t *cmdb.Ticket, expectedDate time.Time) (string, error) {
	if t == nil {
		return "", fmt.Errorf("nil ticket")
	}

	if strings.TrimSpace(t.TicketNumber) == "" {
		return "", fmt.Errorf("empty ticket_number")
	}

	data, err := db.SQLFiles.ReadFile("sql/upsert_secarch_ticket.sql")
	if err != nil {
		return "", fmt.Errorf("read upsert sql: %w", err)
	}

	cmdTag, err := pool.Exec(
		ctx,
		string(data),
		t.TicketNumber,
		t.Summary,
		t.Reporter,
		t.Assignee,
		t.CMDBSystemName,
		t.TicketCreatedAt,
		t.TicketClosedAt,
		expectedDate,
	)

	if err != nil {
		return "", fmt.Errorf("upsert ticket %s: %w", t.TicketNumber, err)
	}

	// Report whether the upsert changed any rows.
	rows := cmdTag.RowsAffected()
	if rows == 0 {
		return "no_change", nil
	}

	return "upserted", nil
}

// ListTickets returns all stored tickets from PostgreSQL.
func ListTickets(ctx context.Context, pool *pgxpool.Pool) ([]StoredTicket, error) {
	data, err := db.SQLFiles.ReadFile("sql/list_secarch_tickets.sql")
	if err != nil {
		return nil, fmt.Errorf("read list tickets sql: %w", err)
	}

	rows, err := pool.Query(ctx, string(data))
	if err != nil {
		return nil, fmt.Errorf("query tickets: %w", err)
	}

	defer rows.Close()

	tickets := make([]StoredTicket, 0)
	for rows.Next() {
		var t StoredTicket
		if err := rows.Scan(
			&t.ID,
			&t.TicketNumber,
			&t.Summary,
			&t.Reporter,
			&t.Assignee,
			&t.CMDBSystemName,
			&t.TicketCreatedAt,
			&t.TicketClosedAt,
			&t.ExpectedDate,
			&t.CreatedAt,
			&t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ticket row: %w", err)
		}
		tickets = append(tickets, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticket rows: %w", err)
	}

	return tickets, nil
}

// refreshResult captures the outcome of one CMDB refresh attempt.
type refreshResult struct {
	ticketNumber string
	status       string
	err          error
}

// RefreshTicketsFromCMDB refreshes stored tickets from CMDB on a best-effort basis.
//
// Notes:
//   - ticket_number comes from the database.
//   - expected_date remains the value stored in the database.
//   - one ticket failure does not fail the whole refresh.
func RefreshTicketsFromCMDB(ctx context.Context, pool *pgxpool.Pool, cmdbClient *cmdb.Client, tickets []StoredTicket) {
	if len(tickets) == 0 {
		return
	}

	resultCh := make(chan refreshResult, len(tickets))
	var wg sync.WaitGroup

	for _, storedTicket := range tickets {
		wg.Add(1)

		go func(st StoredTicket) {
			defer wg.Done()
			if strings.TrimSpace(st.TicketNumber) == "" {
				resultCh <- refreshResult{
					ticketNumber: "",
					err:          fmt.Errorf("skip dirty row: empty ticket_number"),
				}
				return
			}

			latestTicket, err := cmdbClient.GetTicket(ctx, st.TicketNumber)
			if err != nil {
				resultCh <- refreshResult{
					ticketNumber: st.TicketNumber,
					err:          fmt.Errorf("fetch from CMDB failed: %w", err),
				}
				return
			}

			if latestTicket == nil || strings.TrimSpace(latestTicket.TicketNumber) == "" {
				resultCh <- refreshResult{
					ticketNumber: st.TicketNumber,
					err:          fmt.Errorf("invalid ticket returned by CMDB"),
				}
				return
			}

			status, err := UpsertTicket(ctx, pool, latestTicket, st.ExpectedDate)
			if err != nil {
				resultCh <- refreshResult{
					ticketNumber: st.TicketNumber,
					err:          fmt.Errorf("upsert failed: %w", err),
				}
				return
			}

			resultCh <- refreshResult{
				ticketNumber: st.TicketNumber,
				status:       status,
			}
		}(storedTicket)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for result := range resultCh {
		if result.err != nil {
			logger.Error("refresh ticket failed", "ticket_number", result.ticketNumber, "err", result.err)
			continue
		}
		logger.Info("refresh ticket success", "ticket_number", result.ticketNumber, "status", result.status)
	}
}

// UpdateExpectedDate updates expected_date for a ticket.
func UpdateExpectedDate(ctx context.Context, pool *pgxpool.Pool, ticketNumber string, expectedDate time.Time) error {
	cmd, err := pool.Exec(
		ctx, `
		UPDATE secarch_tickets.tickets
		SET expected_date = $1,
		    updated_at = NOW()
		WHERE ticket_number = $2
		`,
		expectedDate,
		ticketNumber,
	)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("ticket not found")
	}

	return nil
}

// DeleteTicket deletes a ticket by ticket number.
func DeleteTicket(ctx context.Context, pool *pgxpool.Pool, ticketNumber string) error {
	cmd, err := pool.Exec(
		ctx, `
		DELETE FROM secarch_tickets.tickets
		WHERE ticket_number = $1
		`, ticketNumber,
	)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("ticket not found")
	}

	return nil
}
