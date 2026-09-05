package secarch

import (
	"context"
	"strings"
	"time"

	"secarch-tickets/internal/cmdb"

	"github.com/jackc/pgx/v5/pgxpool"
)

type cmdbTicketClient interface {
	ListTickets(context.Context, string) ([]*cmdb.Ticket, error)
	ResolveDepartments(context.Context, []cmdb.SystemReference) (map[string]string, error)
}

// RefreshPolicy contains server-side refresh throttling and circuit settings.
type RefreshPolicy struct {
	Timeout                  time.Duration
	SuccessCooldown          time.Duration
	FailureBackoff           time.Duration
	FailureBackoffMultiplier int
	CircuitBreakerThreshold  int
	CircuitOpenDuration      time.Duration
	TicketKeyBatchSize       int
}

// TicketService coordinates ticket reads, CMDB synchronization, and local edits.
type TicketService struct {
	pool       *pgxpool.Pool
	cmdbClient cmdbTicketClient
	policy     RefreshPolicy
	now        func() time.Time
	refresh    refreshCoordinator
}

// NewTicketService creates the shared service used by every ticket handler.
func NewTicketService(pool *pgxpool.Pool, cmdbClient cmdbTicketClient, policy RefreshPolicy) *TicketService {
	if policy.TicketKeyBatchSize <= 0 {
		policy.TicketKeyBatchSize = 50
	}
	return &TicketService{
		pool:       pool,
		cmdbClient: cmdbClient,
		policy:     policy,
		now:        time.Now,
	}
}

// ListTickets returns the durable PostgreSQL snapshot without calling CMDB.
func (s *TicketService) ListTickets(ctx context.Context) ([]StoredTicket, error) {
	return ListTickets(ctx, s.pool)
}

// UpdateExpectedDate updates a user-maintained field without changing CMDB.
func (s *TicketService) UpdateExpectedDate(ctx context.Context, ticketNumber string, expectedDate time.Time) error {
	return UpdateExpectedDate(ctx, s.pool, strings.TrimSpace(ticketNumber), expectedDate)
}

// CountClosedTickets returns the number of resolved tickets in a time range.
func (s *TicketService) CountClosedTickets(ctx context.Context, start, endExclusive time.Time) (int64, error) {
	return CountClosedTickets(ctx, s.pool, start, endExclusive)
}
