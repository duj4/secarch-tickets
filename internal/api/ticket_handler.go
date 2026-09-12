package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"secarch-tickets/internal/logger"
	"secarch-tickets/internal/secarch"

	"github.com/gin-gonic/gin"
)

// ListTicketsHandler returns the stored snapshot and refresh-control status.
func ListTicketsHandler(service *secarch.TicketService) gin.HandlerFunc {
	return func(c *gin.Context) {
		tickets, err := service.ListTickets(c.Request.Context(), ticketAccess(c))
		if err != nil {
			logger.Error("list tickets failed", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"tickets": tickets,
			"sync":    service.SyncStatus(),
		})
	}
}

// RefreshTicketsHandler handles an explicit browser-triggered CMDB sync.
func RefreshTicketsHandler(service *secarch.TicketService) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := service.RefreshOpenTickets(c.Request.Context())
		if err == nil {
			c.JSON(http.StatusOK, result)
			return
		}

		statusCode := http.StatusBadGateway
		code := "cmdb_sync_failed"
		message := "CMDB synchronization failed. The existing data remains available."
		if errors.Is(err, secarch.ErrRefreshRejected) {
			statusCode = http.StatusTooManyRequests
			code = "refresh_cooldown"
			message = "A new CMDB synchronization is not allowed yet."
		}
		if result.Sync.Status == "circuit_open" {
			statusCode = http.StatusServiceUnavailable
			code = "cmdb_sync_circuit_open"
			message = "CMDB synchronization has failed repeatedly. Existing data remains available."
		}
		if result.Sync.RetryAfterSeconds > 0 {
			c.Header("Retry-After", strconv.Itoa(result.Sync.RetryAfterSeconds))
		}
		logger.Error("refresh tickets request failed", "status", result.Sync.Status, "err", err)
		c.JSON(statusCode, gin.H{
			"code":    code,
			"message": message,
			"sync":    result.Sync,
		})
	}
}

// UpdateExpectedDateRequest is the expected-date update body.
type UpdateExpectedDateRequest struct {
	ExpectedDate string `json:"expected_date" binding:"required"`
}

// UpdateExpectedDateHandler changes the local expected date for an Open ticket.
func UpdateExpectedDateHandler(service *secarch.TicketService) gin.HandlerFunc {
	return func(c *gin.Context) {
		ticketNumber := strings.TrimSpace(c.Param("ticket_number"))
		var request UpdateExpectedDateRequest
		if ticketNumber == "" || c.ShouldBindJSON(&request) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ticket_number and expected_date are required"})
			return
		}
		expectedDate, err := time.Parse("2006-01-02", request.ExpectedDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "expected_date must be in YYYY-MM-DD format"})
			return
		}
		if err := service.UpdateExpectedDate(c.Request.Context(), ticketNumber, expectedDate, ticketAccess(c)); err != nil {
			if errors.Is(err, secarch.ErrTicketNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
				return
			}
			logger.Error("update expected date failed", "ticket_number", ticketNumber, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ticket_number": ticketNumber, "status": "updated"})
	}
}

// ClosedStatisticsHandler counts tickets resolved in an inclusive date range.
func ClosedStatisticsHandler(service *secarch.TicketService) gin.HandlerFunc {
	return func(c *gin.Context) {
		startText := strings.TrimSpace(c.Query("start"))
		endText := strings.TrimSpace(c.Query("end"))
		startDate, startErr := time.Parse("2006-01-02", startText)
		endDate, endErr := time.Parse("2006-01-02", endText)
		if startErr != nil || endErr != nil || endDate.Before(startDate) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "start and end must be valid YYYY-MM-DD dates and start must not be after end"})
			return
		}

		china := time.FixedZone("Asia/Shanghai", 8*60*60)
		start := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, china)
		endExclusive := time.Date(endDate.Year(), endDate.Month(), endDate.Day()+1, 0, 0, 0, 0, china)
		count, err := service.CountClosedTickets(c.Request.Context(), start, endExclusive, ticketAccess(c))
		if err != nil {
			logger.Error("count Closed tickets failed", "start", startText, "end", endText, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"start_date":   startText,
			"end_date":     endText,
			"closed_count": count,
		})
	}
}
