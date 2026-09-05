-- sql/list_ticket_updates.sql

SELECT
    u.id,
    u.content,
    u.created_at
FROM secarch_tickets.ticket_updates AS u
JOIN secarch_tickets.tickets AS t ON t.id = u.ticket_id
WHERE t.ticket_number = $1
ORDER BY
    u.created_at DESC,
    u.id DESC;
