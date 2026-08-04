-- sql/create_ticket_update.sql

INSERT INTO secarch_tickets.ticket_updates (
    ticket_id,
    content
)
SELECT
    id,
    $2
FROM secarch_tickets.tickets
WHERE ticket_number = $1
RETURNING
    id,
    content,
    created_at;
