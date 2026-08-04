-- sql/list_secarch_tickets.sql

SELECT
    t.id,
    t.ticket_number,
    t.summary,
    t.reporter,
    t.assignee,
    t.cmdb_system_name,
    t.ticket_created_at,
    t.ticket_closed_at,
    t.expected_date,
    t.created_at,
    t.updated_at,
    COALESCE(u.update_count, 0),
    u.latest_update_at
FROM secarch_tickets.tickets AS t
LEFT JOIN (
    SELECT
        ticket_id,
        COUNT(*) AS update_count,
        MAX(created_at) AS latest_update_at
    FROM secarch_tickets.ticket_updates
    GROUP BY ticket_id
) AS u ON u.ticket_id = t.id
ORDER BY
    t.expected_date ASC NULLS LAST,
    t.updated_at ASC
