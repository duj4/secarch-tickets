package api

import (
	"secarch-tickets/internal/oidcauth"
	"secarch-tickets/internal/secarch"

	"github.com/gin-gonic/gin"
)

func ticketAccess(c *gin.Context) secarch.TicketAccess {
	principal, ok := oidcauth.PrincipalFromContext(c)
	if !ok {
		return secarch.TicketAccess{}
	}
	return accessForPrincipal(principal)
}

func accessForPrincipal(principal oidcauth.Principal) secarch.TicketAccess {
	return secarch.TicketAccess{
		All:      principal.IsAdmin,
		Reporter: principal.Username,
	}
}
