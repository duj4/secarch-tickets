package secarch

import (
	"testing"
	"time"
)

func TestDefaultExpectedDateUsesOneCalendarMonthAndClamps(t *testing.T) {
	china := time.FixedZone("Asia/Shanghai", 8*60*60)
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "ordinary", now: time.Date(2026, time.September, 6, 16, 30, 0, 0, china), want: "2026-10-06"},
		{name: "month end", now: time.Date(2026, time.January, 31, 12, 0, 0, 0, china), want: "2026-02-28"},
		{name: "leap year", now: time.Date(2028, time.January, 31, 12, 0, 0, 0, china), want: "2028-02-29"},
		{name: "year boundary", now: time.Date(2026, time.December, 31, 12, 0, 0, 0, china), want: "2027-01-31"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := defaultExpectedDate(test.now).Format("2006-01-02"); got != test.want {
				t.Fatalf("defaultExpectedDate() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestRefreshFailureBackoffOpensOnThirdFailure(t *testing.T) {
	base := time.Date(2026, time.September, 6, 10, 0, 0, 0, time.UTC)
	current := base
	service := &TicketService{
		policy: RefreshPolicy{
			FailureBackoff: 10 * time.Second, FailureBackoffMultiplier: 2,
			CircuitBreakerThreshold: 3, CircuitOpenDuration: 5 * time.Minute,
		},
		now: func() time.Time { return current },
	}

	first := service.recordRefreshFailure(current)
	if first.Sync.Status != "backoff" || first.Sync.RetryAfterSeconds != 10 {
		t.Fatalf("first failure = %#v", first.Sync)
	}
	current = current.Add(10 * time.Second)
	second := service.recordRefreshFailure(current)
	if second.Sync.Status != "backoff" || second.Sync.RetryAfterSeconds != 20 {
		t.Fatalf("second failure = %#v", second.Sync)
	}
	current = current.Add(20 * time.Second)
	third := service.recordRefreshFailure(current)
	if third.Sync.Status != "circuit_open" || third.Sync.RetryAfterSeconds != 300 {
		t.Fatalf("third failure = %#v", third.Sync)
	}

	current = current.Add(5 * time.Minute)
	status := service.SyncStatus()
	if status.Status != "half_open" || status.RetryAfterSeconds != 0 {
		t.Fatalf("status after open interval = %#v", status)
	}
}
