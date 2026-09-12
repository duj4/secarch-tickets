package secarch

import (
	"errors"
	"strings"
)

var ErrAccessIdentityRequired = errors.New("ticket access identity is required")

// TicketAccess describes the rows the authenticated caller may access. Admin
// callers have All set; every other caller is constrained to their reporter
// identity regardless of Department.
type TicketAccess struct {
	All      bool
	Reporter string
}

func (a TicketAccess) queryArguments() (bool, string, error) {
	if a.All {
		return true, "", nil
	}
	reporter := strings.TrimSpace(a.Reporter)
	if reporter == "" {
		return false, "", ErrAccessIdentityRequired
	}
	return false, reporter, nil
}
