-- sql/upsert_secarch_ticket.sql

INSERT INTO secarch_tickets.tickets (
    ticket_number,
    summary,
    reporter,
    assignee,
    cmdb_system_name,
    ticket_created_at,
    ticket_closed_at,
    expected_date,
    created_at,
    updated_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    NOW(),
    NOW()
)
ON CONFLICT (ticket_number)
DO UPDATE SET
    summary           = EXCLUDED.summary,
    reporter          = EXCLUDED.reporter,
    assignee          = EXCLUDED.assignee,
    cmdb_system_name  = EXCLUDED.cmdb_system_name,
    ticket_created_at = EXCLUDED.ticket_created_at,
    ticket_closed_at  = EXCLUDED.ticket_closed_at,
    expected_date     = EXCLUDED.expected_date,
    updated_at        = NOW()
WHERE
    secarch_tickets.tickets.summary IS DISTINCT FROM EXCLUDED.summary OR
    secarch_tickets.tickets.reporter IS DISTINCT FROM EXCLUDED.reporter OR
    secarch_tickets.tickets.assignee IS DISTINCT FROM EXCLUDED.assignee OR
    secarch_tickets.tickets.cmdb_system_name IS DISTINCT FROM EXCLUDED.cmdb_system_name OR
    secarch_tickets.tickets.ticket_created_at IS DISTINCT FROM EXCLUDED.ticket_created_at OR
    secarch_tickets.tickets.ticket_closed_at IS DISTINCT FROM EXCLUDED.ticket_closed_at OR
    secarch_tickets.tickets.expected_date IS DISTINCT FROM EXCLUDED.expected_date;
