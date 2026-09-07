package secarch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"secarch-tickets/internal/cmdb"
	"secarch-tickets/internal/logger"
)

var ErrRefreshRejected = errors.New("refresh rejected")

// SyncStatus is safe to expose to the browser alongside the stored ticket data.
type SyncStatus struct {
	Status              string     `json:"status"`
	LastSuccessAt       *time.Time `json:"last_success_at"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	RetryAfterSeconds   int        `json:"retry_after_seconds"`
}

// RefreshResult describes whether a user-triggered request reached CMDB.
type RefreshResult struct {
	Sync         SyncStatus `json:"sync"`
	Attempted    bool       `json:"attempted"`
	UpdatedCount int        `json:"updated_count"`
}

type refreshCoordinator struct {
	mu                  sync.Mutex
	inFlight            *refreshFlight
	lastSuccessAt       *time.Time
	nextAllowedAt       time.Time
	circuitOpenUntil    time.Time
	consecutiveFailures int
	status              string
}

type refreshFlight struct {
	done   chan struct{}
	result RefreshResult
	err    error
}

// Bootstrap performs the required full Closed + Open synchronization.
func (s *TicketService) Bootstrap(ctx context.Context) error {
	if s.cmdbClient == nil {
		return fmt.Errorf("CMDB client is nil")
	}
	syncCtx, cancel := context.WithTimeout(ctx, s.policy.Timeout)
	defer cancel()

	updatedCount, err := s.syncTickets(syncCtx, false, true)
	if err != nil {
		return fmt.Errorf("bootstrap CMDB synchronization: %w", err)
	}
	now := s.now()
	s.refresh.mu.Lock()
	s.refresh.lastSuccessAt = timePointer(now)
	s.refresh.nextAllowedAt = now.Add(s.policy.SuccessCooldown)
	s.refresh.consecutiveFailures = 0
	s.refresh.status = "cooldown"
	s.refresh.mu.Unlock()
	logger.Info("bootstrap CMDB synchronization completed", "updated_count", updatedCount)
	return nil
}

// RefreshOpenTickets runs or joins one user-triggered Open-ticket sync.
func (s *TicketService) RefreshOpenTickets(ctx context.Context) (RefreshResult, error) {
	s.refresh.mu.Lock()
	flight := s.refresh.inFlight
	leader := false
	if flight == nil {
		flight = &refreshFlight{done: make(chan struct{})}
		s.refresh.inFlight = flight
		leader = true
	}
	s.refresh.mu.Unlock()

	if !leader {
		return s.waitForRefresh(ctx, flight)
	}

	result, err := s.refreshOpenTicketsOnce()
	s.refresh.mu.Lock()
	flight.result = result
	flight.err = err
	if s.refresh.inFlight == flight {
		s.refresh.inFlight = nil
	}
	close(flight.done)
	s.refresh.mu.Unlock()
	return result, err
}

func (s *TicketService) waitForRefresh(ctx context.Context, flight *refreshFlight) (RefreshResult, error) {
	select {
	case <-ctx.Done():
		return RefreshResult{Sync: s.SyncStatus()}, ctx.Err()
	case <-flight.done:
		return flight.result, flight.err
	}
}

func (s *TicketService) refreshOpenTicketsOnce() (RefreshResult, error) {
	now := s.now()
	if result, rejected := s.rejectRefresh(now); rejected {
		return result, ErrRefreshRejected
	}

	syncCtx, cancel := context.WithTimeout(context.Background(), s.policy.Timeout)
	defer cancel()
	updatedCount, err := s.syncTickets(syncCtx, true, false)
	completedAt := s.now()
	if err != nil {
		result := s.recordRefreshFailure(completedAt)
		logger.Error(
			"CMDB synchronization failed",
			"consecutive_failures", result.Sync.ConsecutiveFailures,
			"status", result.Sync.Status,
			"err", err,
		)
		return result, err
	}

	result := s.recordRefreshSuccess(completedAt, updatedCount)
	logger.Info("CMDB synchronization completed", "updated_count", updatedCount)
	return result, nil
}

func (s *TicketService) rejectRefresh(now time.Time) (RefreshResult, bool) {
	s.refresh.mu.Lock()
	defer s.refresh.mu.Unlock()

	if !s.refresh.circuitOpenUntil.IsZero() {
		if now.Before(s.refresh.circuitOpenUntil) {
			s.refresh.status = "circuit_open"
			return RefreshResult{Sync: s.syncStatusLocked(now)}, true
		}
		s.refresh.status = "half_open"
		return RefreshResult{}, false
	}

	if now.Before(s.refresh.nextAllowedAt) {
		if s.refresh.consecutiveFailures > 0 {
			s.refresh.status = "backoff"
		} else {
			s.refresh.status = "cooldown"
		}
		return RefreshResult{Sync: s.syncStatusLocked(now)}, true
	}
	return RefreshResult{}, false
}

func (s *TicketService) recordRefreshSuccess(now time.Time, updatedCount int) RefreshResult {
	s.refresh.mu.Lock()
	defer s.refresh.mu.Unlock()
	s.refresh.lastSuccessAt = timePointer(now)
	s.refresh.nextAllowedAt = now.Add(s.policy.SuccessCooldown)
	s.refresh.circuitOpenUntil = time.Time{}
	s.refresh.consecutiveFailures = 0
	s.refresh.status = "cooldown"
	return RefreshResult{
		Sync:         s.syncStatusLocked(now),
		Attempted:    true,
		UpdatedCount: updatedCount,
	}
}

func (s *TicketService) recordRefreshFailure(now time.Time) RefreshResult {
	s.refresh.mu.Lock()
	defer s.refresh.mu.Unlock()
	s.refresh.consecutiveFailures++
	if s.refresh.consecutiveFailures >= s.policy.CircuitBreakerThreshold {
		s.refresh.status = "circuit_open"
		s.refresh.circuitOpenUntil = now.Add(s.policy.CircuitOpenDuration)
		s.refresh.nextAllowedAt = time.Time{}
	} else {
		s.refresh.status = "backoff"
		exponent := s.refresh.consecutiveFailures - 1
		multiplier := math.Pow(float64(s.policy.FailureBackoffMultiplier), float64(exponent))
		delay := time.Duration(float64(s.policy.FailureBackoff) * multiplier)
		s.refresh.nextAllowedAt = now.Add(delay)
	}
	return RefreshResult{Sync: s.syncStatusLocked(now), Attempted: true}
}

// SyncStatus returns the current refresh-control state without accessing CMDB.
func (s *TicketService) SyncStatus() SyncStatus {
	now := s.now()
	s.refresh.mu.Lock()
	defer s.refresh.mu.Unlock()
	if s.refresh.status == "circuit_open" && !now.Before(s.refresh.circuitOpenUntil) {
		s.refresh.status = "half_open"
	}
	if (s.refresh.status == "cooldown" || s.refresh.status == "backoff") && !now.Before(s.refresh.nextAllowedAt) {
		s.refresh.status = "ready"
	}
	return s.syncStatusLocked(now)
}

func (s *TicketService) syncStatusLocked(now time.Time) SyncStatus {
	retryAt := s.refresh.nextAllowedAt
	if s.refresh.status == "circuit_open" {
		retryAt = s.refresh.circuitOpenUntil
	}
	retryAfter := 0
	if now.Before(retryAt) {
		retryAfter = int(math.Ceil(retryAt.Sub(now).Seconds()))
	}
	return SyncStatus{
		Status:              defaultStatus(s.refresh.status),
		LastSuccessAt:       cloneTimePointer(s.refresh.lastSuccessAt),
		ConsecutiveFailures: s.refresh.consecutiveFailures,
		RetryAfterSeconds:   retryAfter,
	}
}

func (s *TicketService) syncTickets(ctx context.Context, openOnly, resolveAllSystems bool) (int, error) {
	ownershipCandidates, err := listTicketOwnershipCandidates(ctx, s.pool, openOnly)
	if err != nil {
		return 0, err
	}
	logger.Info(
		"stored ticket ownership repair scan completed",
		"open_only", openOnly,
		"candidate_count", len(ownershipCandidates),
	)

	var openBefore []string
	knownDepartments := make(map[string]string)
	if openOnly {
		openBefore, err = ListOpenTicketNumbers(ctx, s.pool)
		if err != nil {
			return 0, err
		}
		knownDepartments, err = ListKnownDepartments(ctx, s.pool)
		if err != nil {
			return 0, err
		}
	}

	tickets, err := s.cmdbClient.ListTickets(ctx, cmdb.SecArchTicketJQL(openOnly))
	if err != nil {
		return 0, err
	}

	if openOnly {
		openKeys := make(map[string]struct{}, len(tickets))
		for _, ticket := range tickets {
			openKeys[ticket.TicketNumber] = struct{}{}
		}
		missing := make([]string, 0)
		for _, key := range openBefore {
			if _, ok := openKeys[key]; !ok {
				missing = append(missing, key)
			}
		}
		tracked, err := s.fetchMissingTrackedTickets(ctx, missing)
		if err != nil {
			return 0, err
		}
		tickets = append(tickets, tracked...)
	}

	if err := s.attachDepartments(ctx, tickets, knownDepartments, resolveAllSystems); err != nil {
		return 0, err
	}
	repairs, err := s.prepareStoredTicketOwnershipRepairs(ctx, ownershipCandidates)
	if err != nil {
		return 0, err
	}
	persistence, err := persistTicketSync(
		ctx,
		s.pool,
		tickets,
		defaultExpectedDate(s.now()),
		repairs,
	)
	if err != nil {
		return 0, err
	}
	for _, ticketNumber := range persistence.SkippedRepairTickets {
		logger.Warn(
			"stored ticket ownership repair skipped because ownership changed or is already current",
			"ticket_number", ticketNumber,
		)
	}
	logger.Info(
		"stored ticket ownership repair completed",
		"candidate_count", len(ownershipCandidates),
		"prepared_count", len(repairs),
		"applied_count", persistence.AppliedRepairCount,
		"skipped_count", len(persistence.SkippedRepairTickets),
	)
	return persistence.UpdatedCount, nil
}

// prepareStoredTicketOwnershipRepairs self-heals rows created before System
// keys were normalized. It uses the full System value already stored with the
// ticket, so it does not depend on the ticket queue API returning that ticket.
func (s *TicketService) prepareStoredTicketOwnershipRepairs(ctx context.Context, candidates []ticketOwnershipCandidate) ([]ticketOwnershipRepair, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	type preparedRepair struct {
		candidate ticketOwnershipCandidate
		system    cmdb.SystemReference
	}
	prepared := make([]preparedRepair, 0, len(candidates))
	references := make([]cmdb.SystemReference, 0, len(candidates))
	for _, candidate := range candidates {
		system, err := storedTicketSystemReference(candidate)
		if err != nil {
			logger.Warn(
				"stored ticket System cannot be normalized; ownership repair skipped",
				"ticket_number", candidate.TicketNumber,
				"err", err,
			)
			continue
		}
		if system.Key == "" {
			continue
		}
		prepared = append(prepared, preparedRepair{candidate: candidate, system: system})
		references = append(references, system)
	}
	if len(prepared) == 0 {
		return nil, nil
	}

	departments, err := s.resolveDepartmentsIsolated(ctx, references)
	if err != nil {
		return nil, fmt.Errorf("resolve stored ticket System ownership: %w", err)
	}

	repairs := make([]ticketOwnershipRepair, 0, len(prepared))
	for _, item := range prepared {
		department := strings.TrimSpace(departments[item.system.Key])
		if department == "" {
			logger.Warn(
				"stored ticket ownership repair skipped because System could not be resolved",
				"ticket_number", item.candidate.TicketNumber,
				"system_key", item.system.Key,
			)
			continue
		}
		systemNames := item.candidate.CMDBSystemName
		if item.system.Name != "" {
			systemNames = []string{item.system.Name}
		}
		if ticketOwnershipMatches(item.candidate, systemNames, item.system.Key, department) {
			continue
		}
		repairs = append(repairs, ticketOwnershipRepair{
			TicketNumber:           item.candidate.TicketNumber,
			ExpectedCMDBSystemName: item.candidate.CMDBSystemName,
			ExpectedCMDBSystemKey:  item.candidate.CMDBSystemKey,
			ExpectedDepartment:     item.candidate.Department,
			CMDBSystemName:         systemNames,
			CMDBSystemKey:          item.system.Key,
			Department:             department,
		})
		logger.Info(
			"prepared stored ticket ownership repair",
			"ticket_number", item.candidate.TicketNumber,
			"system_key", item.system.Key,
			"system_name", item.system.Name,
			"department", department,
		)
	}
	return repairs, nil
}

func (s *TicketService) resolveDepartmentsIsolated(ctx context.Context, references []cmdb.SystemReference) (map[string]string, error) {
	unique := make(map[string]cmdb.SystemReference, len(references))
	for _, reference := range references {
		if existing, ok := unique[reference.Key]; !ok || existing.Name == "" {
			unique[reference.Key] = reference
		}
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make([]cmdb.SystemReference, 0, len(keys))
	for _, key := range keys {
		ordered = append(ordered, unique[key])
	}

	resolved := make(map[string]string, len(ordered))
	var resolveSubset func([]cmdb.SystemReference) error
	resolveSubset = func(subset []cmdb.SystemReference) error {
		if len(subset) == 0 {
			return nil
		}
		departments, err := s.cmdbClient.ResolveDepartments(ctx, subset)
		if err == nil {
			for key, department := range departments {
				resolved[key] = department
			}
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !cmdb.IsSystemResolutionDataError(err) {
			return err
		}
		if len(subset) == 1 {
			logger.Warn(
				"CMDB System cannot be resolved from its data; affected tickets skipped",
				"system_key", subset[0].Key,
				"system_name", subset[0].Name,
				"err", err,
			)
			return nil
		}

		logger.Warn(
			"CMDB System data error found in batch; isolating Systems",
			"system_count", len(subset),
			"err", err,
		)
		middle := len(subset) / 2
		if err := resolveSubset(subset[:middle]); err != nil {
			return err
		}
		return resolveSubset(subset[middle:])
	}
	if err := resolveSubset(ordered); err != nil {
		return nil, err
	}
	return resolved, nil
}

func ticketOwnershipMatches(candidate ticketOwnershipCandidate, systemNames []string, systemKey, department string) bool {
	if candidate.CMDBSystemKey != systemKey || candidate.Department != department {
		return false
	}
	if (candidate.CMDBSystemName == nil) != (systemNames == nil) || len(candidate.CMDBSystemName) != len(systemNames) {
		return false
	}
	for index := range candidate.CMDBSystemName {
		if candidate.CMDBSystemName[index] != systemNames[index] {
			return false
		}
	}
	return true
}

func storedTicketSystemReference(candidate ticketOwnershipCandidate) (cmdb.SystemReference, error) {
	existingKey := strings.ToUpper(strings.TrimSpace(candidate.CMDBSystemKey))
	if existingKey == "" {
		return cmdb.ParseTicketSystemReference(candidate.CMDBSystemName)
	}

	keyReference, err := cmdb.ParseTicketSystemReference([]string{existingKey})
	if err != nil {
		return cmdb.SystemReference{}, fmt.Errorf("invalid stored System key %q: %w", candidate.CMDBSystemKey, err)
	}
	system := cmdb.SystemReference{Key: keyReference.Key}

	names := make([]string, 0, len(candidate.CMDBSystemName))
	for _, name := range candidate.CMDBSystemName {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	if len(names) != 1 {
		return system, nil
	}

	parsed, parseErr := cmdb.ParseTicketSystemReference(names)
	if parseErr != nil {
		system.Name = names[0]
		return system, nil
	}
	if parsed.Key != "" && parsed.Key != system.Key {
		return cmdb.SystemReference{}, fmt.Errorf(
			"stored System key %s conflicts with key %s in display value",
			system.Key,
			parsed.Key,
		)
	}
	system.Name = parsed.Name
	return system, nil
}

// fetchMissingTrackedTickets refreshes locally tracked Open tickets that were
// absent from the broad Open-ticket query. The exact lookup may show that a
// ticket is either still Open or now Closed; both results must be synchronized.
func (s *TicketService) fetchMissingTrackedTickets(ctx context.Context, candidates []string) ([]*cmdb.Ticket, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Strings(candidates)
	tracked := make([]*cmdb.Ticket, 0, len(candidates))
	for start := 0; start < len(candidates); start += s.policy.TicketKeyBatchSize {
		end := min(start+s.policy.TicketKeyBatchSize, len(candidates))
		jql, err := cmdb.TicketKeysJQL(candidates[start:end])
		if err != nil {
			return nil, err
		}
		results, err := s.cmdbClient.ListTickets(ctx, jql)
		if err != nil {
			logger.Warn(
				"CMDB JQL lookup for tracked tickets failed; retrying individually by issueKey",
				"ticket_count", end-start,
				"err", err,
			)
			results = nil
		}
		tracked = append(tracked, results...)

		returned := make(map[string]struct{}, len(results))
		for _, ticket := range results {
			if ticket != nil {
				returned[strings.ToUpper(strings.TrimSpace(ticket.TicketNumber))] = struct{}{}
			}
		}
		for _, candidate := range candidates[start:end] {
			candidate = strings.ToUpper(strings.TrimSpace(candidate))
			if _, ok := returned[candidate]; ok {
				continue
			}
			logger.Warn(
				"CMDB JQL lookup omitted tracked ticket; retrying by issueKey",
				"ticket_number", candidate,
			)
			ticket, err := s.cmdbClient.GetTicket(ctx, candidate)
			if errors.Is(err, cmdb.ErrTicketNotFound) {
				logger.Warn("CMDB no longer returns tracked ticket", "ticket_number", candidate)
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("refresh tracked ticket %s by issueKey: %w", candidate, err)
			}
			tracked = append(tracked, ticket)
		}
	}
	return tracked, nil
}

func (s *TicketService) attachDepartments(ctx context.Context, tickets []*cmdb.Ticket, known map[string]string, resolveAll bool) error {
	references := make(map[string]cmdb.SystemReference)
	for _, ticket := range tickets {
		if ticket == nil || ticket.CMDBSystemKey == "" {
			continue
		}
		if !resolveAll {
			if department := known[ticket.CMDBSystemKey]; department != "" && department != cmdb.UnassignedDepartment {
				ticket.Department = department
				continue
			}
		}
		references[ticket.CMDBSystemKey] = cmdb.SystemReference{
			Key:  ticket.CMDBSystemKey,
			Name: ticket.CMDBSystemLabel,
		}
	}

	toResolve := make([]cmdb.SystemReference, 0, len(references))
	for _, reference := range references {
		toResolve = append(toResolve, reference)
	}
	resolved, err := s.resolveDepartmentsIsolated(ctx, toResolve)
	if err != nil {
		return fmt.Errorf("resolve ticket System Departments: %w", err)
	}
	for _, ticket := range tickets {
		if ticket == nil || ticket.CMDBSystemKey == "" {
			continue
		}
		if department := resolved[ticket.CMDBSystemKey]; department != "" {
			ticket.Department = department
		} else if ticket.Department == "" {
			ticket.Department = known[ticket.CMDBSystemKey]
		}
	}
	return nil
}

func defaultExpectedDate(now time.Time) time.Time {
	china := time.FixedZone("Asia/Shanghai", 8*60*60)
	now = now.In(china)
	year, month, day := now.Date()
	targetMonth := month + 1
	lastDay := time.Date(year, targetMonth+1, 0, 0, 0, 0, 0, china).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, targetMonth, day, 0, 0, 0, 0, china)
}

func defaultStatus(status string) string {
	if status == "" {
		return "ready"
	}
	return status
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
