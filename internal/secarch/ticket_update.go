package secarch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"secarch-tickets/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TicketUpdateMaxLength = 500

var (
	ErrTicketNotFound        = errors.New("ticket not found")
	ErrUpdateContentRequired = errors.New("update content is required")
	ErrUpdateContentTooLong  = errors.New("update content exceeds 500 characters")
)

// TicketUpdate is a local, append-only progress note for a tracked ticket.
type TicketUpdate struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// AddTicketUpdate validates and stores a local update without changing the source ticket.
func (s *TicketService) AddTicketUpdate(ctx context.Context, ticketNumber, content string) (TicketUpdate, error) {
	ticketNumber = strings.TrimSpace(ticketNumber)
	if ticketNumber == "" {
		return TicketUpdate{}, ErrTicketNotFound
	}

	normalized, err := normalizeTicketUpdateContent(content)
	if err != nil {
		return TicketUpdate{}, err
	}

	return createTicketUpdate(ctx, s.pool, ticketNumber, normalized)
}

// ListTicketUpdates returns local updates newest first.
func (s *TicketService) ListTicketUpdates(ctx context.Context, ticketNumber string) ([]TicketUpdate, error) {
	ticketNumber = strings.TrimSpace(ticketNumber)
	if ticketNumber == "" {
		return nil, ErrTicketNotFound
	}

	return listTicketUpdates(ctx, s.pool, ticketNumber)
}

func normalizeTicketUpdateContent(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", ErrUpdateContentRequired
	}
	if utf8.RuneCountInString(content) > TicketUpdateMaxLength {
		return "", ErrUpdateContentTooLong
	}

	return content, nil
}

func createTicketUpdate(ctx context.Context, pool *pgxpool.Pool, ticketNumber, content string) (TicketUpdate, error) {
	data, err := db.SQLFiles.ReadFile("sql/create_ticket_update.sql")
	if err != nil {
		return TicketUpdate{}, fmt.Errorf("read create ticket update sql: %w", err)
	}

	var update TicketUpdate
	err = pool.QueryRow(ctx, string(data), ticketNumber, content).Scan(
		&update.ID,
		&update.Content,
		&update.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return TicketUpdate{}, ErrTicketNotFound
	}
	if err != nil {
		return TicketUpdate{}, fmt.Errorf("create update for ticket %s: %w", ticketNumber, err)
	}

	return update, nil
}

func listTicketUpdates(ctx context.Context, pool *pgxpool.Pool, ticketNumber string) ([]TicketUpdate, error) {
	var exists bool
	if err := pool.QueryRow(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM secarch_tickets.tickets WHERE ticket_number = $1)",
		ticketNumber,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check ticket %s: %w", ticketNumber, err)
	}
	if !exists {
		return nil, ErrTicketNotFound
	}

	data, err := db.SQLFiles.ReadFile("sql/list_ticket_updates.sql")
	if err != nil {
		return nil, fmt.Errorf("read list ticket updates sql: %w", err)
	}

	rows, err := pool.Query(ctx, string(data), ticketNumber)
	if err != nil {
		return nil, fmt.Errorf("query updates for ticket %s: %w", ticketNumber, err)
	}
	defer rows.Close()

	updates := make([]TicketUpdate, 0)
	for rows.Next() {
		var update TicketUpdate
		if err := rows.Scan(&update.ID, &update.Content, &update.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan update for ticket %s: %w", ticketNumber, err)
		}
		updates = append(updates, update)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate updates for ticket %s: %w", ticketNumber, err)
	}

	return updates, nil
}
