package api

import (
	"errors"
	"net/http"
	"strings"

	"secarch-tickets/internal/logger"
	"secarch-tickets/internal/secarch"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateTicketUpdateRequest is the request body for a local ticket update.
type CreateTicketUpdateRequest struct {
	Content string `json:"content" binding:"required"`
}

// ListTicketUpdatesHandler returns local updates for one ticket.
func ListTicketUpdatesHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	service := secarch.NewTicketService(pool, nil)

	return func(c *gin.Context) {
		ticketNumber := strings.TrimSpace(c.Param("ticket_number"))
		updates, err := service.ListTicketUpdates(c.Request.Context(), ticketNumber)
		if err != nil {
			if errors.Is(err, secarch.ErrTicketNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
				return
			}

			logger.Error("list ticket updates failed", "ticket_number", ticketNumber, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"updates": updates})
	}
}

// CreateTicketUpdateHandler appends a local update to one ticket.
func CreateTicketUpdateHandler(pool *pgxpool.Pool) gin.HandlerFunc {
	service := secarch.NewTicketService(pool, nil)

	return func(c *gin.Context) {
		ticketNumber := strings.TrimSpace(c.Param("ticket_number"))

		var req CreateTicketUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "update content is required"})
			return
		}

		update, err := service.AddTicketUpdate(c.Request.Context(), ticketNumber, req.Content)
		if err != nil {
			switch {
			case errors.Is(err, secarch.ErrTicketNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found"})
			case errors.Is(err, secarch.ErrUpdateContentRequired), errors.Is(err, secarch.ErrUpdateContentTooLong):
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			default:
				logger.Error("create ticket update failed", "ticket_number", ticketNumber, "err", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			}
			return
		}

		logger.Info("ticket update created", "ticket_number", ticketNumber, "update_id", update.ID)
		c.JSON(http.StatusCreated, gin.H{"update": update})
	}
}
