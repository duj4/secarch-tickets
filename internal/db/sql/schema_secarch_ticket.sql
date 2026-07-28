-- sql/schema_secarch_ticket.sql

CREATE TABLE IF NOT EXISTS secarch_tickets.tickets (
    id BIGSERIAL PRIMARY KEY,

    ticket_number TEXT NOT NULL UNIQUE,

    summary          TEXT NOT NULL,
    reporter         TEXT NOT NULL,
    assignee         TEXT,
    cmdb_system_name TEXT[],

    ticket_created_at TIMESTAMPTZ NOT NULL,
    ticket_closed_at  TIMESTAMPTZ,

    expected_date DATE NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT tickets_ticket_number_not_empty CHECK (ticket_number <> '')
)
