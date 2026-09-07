package secarch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"secarch-tickets/internal/cmdb"
	"secarch-tickets/internal/db"

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
	CMDBSystemKey   string     `json:"cmdb_system_key"`
	Department      string     `json:"department"`
	TicketCreatedAt time.Time  `json:"ticket_created_at"`
	TicketClosedAt  *time.Time `json:"ticket_closed_at"`
	ExpectedDate    time.Time  `json:"expected_date"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	UpdateCount     int64      `json:"update_count"`
	LatestUpdateAt  *time.Time `json:"latest_update_at"`
}

type ticketOwnershipCandidate struct {
	TicketNumber   string
	CMDBSystemName []string
	CMDBSystemKey  string
	Department     string
}

type ticketOwnershipRepair struct {
	TicketNumber           string
	ExpectedCMDBSystemName []string
	ExpectedCMDBSystemKey  string
	ExpectedDepartment     string
	CMDBSystemName         []string
	CMDBSystemKey          string
	Department             string
}

type ticketSyncPersistenceResult struct {
	UpdatedCount         int
	AppliedRepairCount   int
	SkippedRepairTickets []string
}

// persistTicketSync commits regular ticket upserts and ownership repairs in
// one transaction. The default expected date is only used for inserts, and
// user-maintained dates survive every CMDB refresh.
func persistTicketSync(
	ctx context.Context,
	pool *pgxpool.Pool,
	tickets []*cmdb.Ticket,
	defaultExpectedDate time.Time,
	repairs []ticketOwnershipRepair,
) (ticketSyncPersistenceResult, error) {
	data, err := db.SQLFiles.ReadFile("sql/upsert_secarch_ticket.sql")
	if err != nil {
		return ticketSyncPersistenceResult{}, fmt.Errorf("read upsert SQL: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return ticketSyncPersistenceResult{}, fmt.Errorf("begin ticket synchronization: %w", err)
	}
	defer tx.Rollback(ctx)

	updatedTickets := make(map[string]struct{}, len(tickets)+len(repairs))
	for _, ticket := range tickets {
		if ticket == nil || strings.TrimSpace(ticket.TicketNumber) == "" {
			return ticketSyncPersistenceResult{}, fmt.Errorf("cannot upsert ticket with an empty key")
		}
		commandTag, err := tx.Exec(
			ctx,
			string(data),
			ticket.TicketNumber,
			ticket.Summary,
			ticket.Reporter,
			ticket.Assignee,
			ticket.CMDBSystemName,
			ticket.CMDBSystemKey,
			ticket.Department,
			ticket.TicketCreatedAt,
			ticket.TicketClosedAt,
			defaultExpectedDate,
		)
		if err != nil {
			return ticketSyncPersistenceResult{}, fmt.Errorf("upsert ticket %s: %w", ticket.TicketNumber, err)
		}
		if commandTag.RowsAffected() > 0 {
			updatedTickets[ticket.TicketNumber] = struct{}{}
		}
	}

	result := ticketSyncPersistenceResult{}
	for _, repair := range repairs {
		commandTag, err := tx.Exec(ctx, `
			UPDATE secarch_tickets.tickets
			SET cmdb_system_name = $2,
			    cmdb_system_key = $3,
			    department = $4,
			    updated_at = NOW()
			WHERE ticket_number = $1
			  AND cmdb_system_name IS NOT DISTINCT FROM $5
			  AND cmdb_system_key IS NOT DISTINCT FROM $6
			  AND department IS NOT DISTINCT FROM $7
			  AND (
			      cmdb_system_name IS DISTINCT FROM $2 OR
			      cmdb_system_key IS DISTINCT FROM $3 OR
			      department IS DISTINCT FROM $4
			  )
		`,
			repair.TicketNumber,
			repair.CMDBSystemName,
			repair.CMDBSystemKey,
			repair.Department,
			repair.ExpectedCMDBSystemName,
			repair.ExpectedCMDBSystemKey,
			repair.ExpectedDepartment,
		)
		if err != nil {
			return ticketSyncPersistenceResult{}, fmt.Errorf("repair ticket %s ownership: %w", repair.TicketNumber, err)
		}
		if commandTag.RowsAffected() == 0 {
			result.SkippedRepairTickets = append(result.SkippedRepairTickets, repair.TicketNumber)
			continue
		}
		result.AppliedRepairCount++
		updatedTickets[repair.TicketNumber] = struct{}{}
	}

	if err := tx.Commit(ctx); err != nil {
		return ticketSyncPersistenceResult{}, fmt.Errorf("commit ticket synchronization: %w", err)
	}
	result.UpdatedCount = len(updatedTickets)
	return result, nil
}

// ListTickets returns all stored tickets from PostgreSQL.
func ListTickets(ctx context.Context, pool *pgxpool.Pool) ([]StoredTicket, error) {
	data, err := db.SQLFiles.ReadFile("sql/list_secarch_tickets.sql")
	if err != nil {
		return nil, fmt.Errorf("read list tickets SQL: %w", err)
	}

	rows, err := pool.Query(ctx, string(data))
	if err != nil {
		return nil, fmt.Errorf("query tickets: %w", err)
	}
	defer rows.Close()

	tickets := make([]StoredTicket, 0)
	for rows.Next() {
		var ticket StoredTicket
		if err := rows.Scan(
			&ticket.ID,
			&ticket.TicketNumber,
			&ticket.Summary,
			&ticket.Reporter,
			&ticket.Assignee,
			&ticket.CMDBSystemName,
			&ticket.CMDBSystemKey,
			&ticket.Department,
			&ticket.TicketCreatedAt,
			&ticket.TicketClosedAt,
			&ticket.ExpectedDate,
			&ticket.CreatedAt,
			&ticket.UpdatedAt,
			&ticket.UpdateCount,
			&ticket.LatestUpdateAt,
		); err != nil {
			return nil, fmt.Errorf("scan ticket row: %w", err)
		}
		tickets = append(tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticket rows: %w", err)
	}
	return tickets, nil
}

// ListKnownDepartments returns the latest persisted Department for each System.
func ListKnownDepartments(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT cmdb_system_key, department
		FROM secarch_tickets.tickets
		WHERE cmdb_system_key IS NOT NULL
		  AND cmdb_system_key <> ''
		  AND department IS NOT NULL
		  AND department <> ''
		  AND department <> $1
		GROUP BY cmdb_system_key, department
	`, cmdb.UnassignedDepartment)
	if err != nil {
		return nil, fmt.Errorf("query known System Departments: %w", err)
	}
	defer rows.Close()

	known := make(map[string]string)
	for rows.Next() {
		var key, department string
		if err := rows.Scan(&key, &department); err != nil {
			return nil, fmt.Errorf("scan known System Department: %w", err)
		}
		if existing, ok := known[key]; ok && existing != department {
			return nil, fmt.Errorf("System %s has inconsistent Departments %q and %q", key, existing, department)
		}
		known[key] = department
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate known System Departments: %w", err)
	}
	return known, nil
}

// ListOpenTicketNumbers returns keys currently considered Open locally.
func ListOpenTicketNumbers(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT ticket_number
		FROM secarch_tickets.tickets
		WHERE ticket_closed_at IS NULL
		ORDER BY ticket_number
	`)
	if err != nil {
		return nil, fmt.Errorf("query Open ticket numbers: %w", err)
	}
	defer rows.Close()

	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan Open ticket number: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Open ticket numbers: %w", err)
	}
	return keys, nil
}

// listTicketOwnershipCandidates returns rows whose System key or Department
// still needs to be normalized or resolved.
func listTicketOwnershipCandidates(ctx context.Context, pool *pgxpool.Pool, openOnly bool) ([]ticketOwnershipCandidate, error) {
	rows, err := pool.Query(ctx, `
		SELECT ticket_number, cmdb_system_name, cmdb_system_key, department
		FROM secarch_tickets.tickets
		WHERE (
		        BTRIM(cmdb_system_key) = '' OR
		        BTRIM(department) = '' OR
		        department = $2
		      )
		  AND (
		        BTRIM(cmdb_system_key) <> '' OR
		        COALESCE(cardinality(cmdb_system_name), 0) > 0
		      )
		  AND (NOT $1::boolean OR ticket_closed_at IS NULL)
		ORDER BY ticket_number
	`, openOnly, cmdb.UnassignedDepartment)
	if err != nil {
		return nil, fmt.Errorf("query ticket ownership repair candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]ticketOwnershipCandidate, 0)
	for rows.Next() {
		var candidate ticketOwnershipCandidate
		if err := rows.Scan(
			&candidate.TicketNumber,
			&candidate.CMDBSystemName,
			&candidate.CMDBSystemKey,
			&candidate.Department,
		); err != nil {
			return nil, fmt.Errorf("scan ticket ownership repair candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticket ownership repair candidates: %w", err)
	}
	return candidates, nil
}

// CountClosedTickets returns tickets resolved in [start, endExclusive).
func CountClosedTickets(ctx context.Context, pool *pgxpool.Pool, start, endExclusive time.Time) (int64, error) {
	data, err := db.SQLFiles.ReadFile("sql/count_closed_tickets.sql")
	if err != nil {
		return 0, fmt.Errorf("read closed ticket count SQL: %w", err)
	}
	var count int64
	if err := pool.QueryRow(ctx, string(data), start, endExclusive).Scan(&count); err != nil {
		return 0, fmt.Errorf("count Closed tickets: %w", err)
	}
	return count, nil
}

// UpdateExpectedDate updates expected_date for a ticket.
func UpdateExpectedDate(ctx context.Context, pool *pgxpool.Pool, ticketNumber string, expectedDate time.Time) error {
	cmd, err := pool.Exec(ctx, `
		UPDATE secarch_tickets.tickets
		SET expected_date = $1,
		    updated_at = NOW()
		WHERE ticket_number = $2
	`, expectedDate, ticketNumber)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrTicketNotFound
	}
	return nil
}
