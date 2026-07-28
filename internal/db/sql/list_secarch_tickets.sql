-- sql/list_secarch_tickets.sql

SELECT
    id,
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
FROM secarch_tickets.tickets
ORDER BY
    expected_date ASC NULLS LAST,
    updated_at ASC
