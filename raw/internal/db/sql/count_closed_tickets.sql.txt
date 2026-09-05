SELECT COUNT(*)
FROM secarch_tickets.tickets
WHERE ticket_closed_at >= $1
  AND ticket_closed_at < $2;
