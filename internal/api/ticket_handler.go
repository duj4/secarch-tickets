package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"secarch-tickets/internal/cmdb"
	"secarch-tickets/internal/logger"
	"secarch-tickets/internal/secarch"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateTicketRequest is the request body for POST /api/tickets.
type CreateTicketRequest struct {
	TicketNumber string `json:"ticket_number" binding:"required"`
	ExpectedDate string `json:"expected_date" binding:"required"`
}

// CreateTicketHandler handles ticket creation requests.
func CreateTicketHandler(pool *pgxpool.Pool, cmdbClient *cmdb.Client) gin.HandlerFunc {
	// Initialize the service once when the handler is registered.
	service := secarch.NewTicketService(pool, cmdbClient)

	return func(c *gin.Context) {
		// Parse the request body.
		var req CreateTicketRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.Error("invalid request body", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid request body",
			})
			return
		}

		// Validate the ticket number.
		req.TicketNumber = strings.TrimSpace(req.TicketNumber)
		if req.TicketNumber == "" {
			logger.Error("empty ticket_number")
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "ticket_number cannot be empty",
			})
			return
		}

		// Parse the expected date.
		expectedDate, err := time.Parse("2006-01-02", req.ExpectedDate)
		if err != nil {
			logger.Error("invalid expected_date format", "expected_date", req.ExpectedDate, "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "expected_date must be in YYYY-MM-DD format",
			})
			return
		}

		// Use the request context for downstream calls.
		ctx := c.Request.Context()

		// Execute the business flow.
		logger.Info("create ticket request received", "ticket_number", req.TicketNumber, "expected_date", expectedDate)
		status, err := service.CreateTicket(ctx, req.TicketNumber, expectedDate)
		if err != nil {
			logger.Error("create ticket request failed", "ticket_number", req.TicketNumber, "err", err)
			if strings.Contains(err.Error(), "does not exist") {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("ticket %s not found in CMDB", req.TicketNumber),
				})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "internal error",
				})
			}
			return
		}

		// Return the service result.
		logger.Info("create ticket success", "ticket_number", req.TicketNumber, "status", status)
		c.JSON(http.StatusOK, gin.H{
			"status": status,
		})
	}
}

// ListTicketsHandler handles ticket listing requests.
func ListTicketsHandler(pool *pgxpool.Pool, cmdbClient *cmdb.Client) gin.HandlerFunc {
	// Initialize the service once when the handler is registered.
	service := secarch.NewTicketService(pool, cmdbClient)

	return func(c *gin.Context) {
		// Use the request context for downstream calls.
		ctx := c.Request.Context()

		// Execute the business flow.
		logger.Info("list tickets request received")
		tickets, err := service.ListTickets(ctx)
		if err != nil {
			logger.Error("list tickets failed", "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "internal error",
			})
			return
		}

		// Return the refreshed ticket list.
		logger.Info("list tickets success", "count", len(tickets))
		c.JSON(http.StatusOK, gin.H{
			"tickets": tickets,
		})
	}
}

// UpdateExpectedDateRequest is the request body for updating expected_date.
type UpdateExpectedDateRequest struct {
	ExpectedDate string `json:"expected_date" binding:"required"`
}

// UpdateTicketHandler handles expected_date update requests.
func UpdateTicketHandler(pool *pgxpool.Pool, cmdbClient *cmdb.Client) gin.HandlerFunc {
	// Initialize the service once when the handler is registered.
	service := secarch.NewTicketService(pool, cmdbClient)

	return func(c *gin.Context) {
		// Read the ticket number from the path.
		ticketNumber := strings.TrimSpace(c.Param("ticket_number"))
		if ticketNumber == "" {
			logger.Error("update ticket failed", "err", "empty ticket_number")
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "ticket_number is required",
			})
			return
		}

		// Parse the request body.
		var req UpdateExpectedDateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.Error("update request body failed", "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid request body",
			})
			return
		}

		// Validate the expected date.
		if req.ExpectedDate == "" {
			logger.Error("update ticket failed", "ticket_number", ticketNumber, "err", "empty expected_date")
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "expected_date is required",
			})
			return
		}

		// Parse the expected date.
		expectedDate, err := time.Parse("2006-01-02", req.ExpectedDate)
		if err != nil {
			logger.Error("update ticket failed", "ticket_number", ticketNumber, "expected_date", req.ExpectedDate, "err", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid date",
			})
			return
		}

		// Execute the update.
		logger.Info("update ticket request received", "ticket_number", ticketNumber, "expected_date", expectedDate)
		err = service.UpdateExpectedDate(c.Request.Context(), ticketNumber, expectedDate)
		if err != nil {
			logger.Error("update ticket failed", "ticket_number", ticketNumber, "err", err)
			if err.Error() == "ticket not found" {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "ticket not found",
				})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "internal error",
				})
			}
			return
		}

		// Return success.
		logger.Info("update ticket success", "ticket_number", ticketNumber, "expected_date", expectedDate)
		c.JSON(http.StatusOK, gin.H{
			"ticket_number": ticketNumber,
			"status":        "updated",
		})
	}
}

// DeleteTicketHandler handles ticket deletion requests.
func DeleteTicketHandler(pool *pgxpool.Pool, cmdbClient *cmdb.Client) gin.HandlerFunc {
	// Initialize the service once when the handler is registered.
	service := secarch.NewTicketService(pool, cmdbClient)

	return func(c *gin.Context) {
		// Read the ticket number from the path.
		ticketNumber := strings.TrimSpace(c.Param("ticket_number"))
		if ticketNumber == "" {
			logger.Error("delete ticket failed", "err", "empty ticket number")
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "ticket_number required",
			})
			return
		}

		// Execute the delete.
		logger.Info("delete ticket request received", "ticket_number", ticketNumber)
		err := service.DeleteTicket(c.Request.Context(), ticketNumber)
		if err != nil {
			logger.Error("delete ticket request failed", "ticket_number", ticketNumber, "err", err)

			if err.Error() == "ticket not found" {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "ticket not found",
				})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "internal error",
				})
			}
			return
		}

		// Return success.
		logger.Info("delete ticket success", "ticket_number", ticketNumber)
		c.JSON(http.StatusOK, gin.H{
			"ticket_number": ticketNumber,
			"status":        "deleted",
		})
	}
}
