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
);

CREATE TABLE IF NOT EXISTS secarch_tickets.ticket_updates (
    id BIGSERIAL PRIMARY KEY,

    ticket_id BIGINT NOT NULL
        REFERENCES secarch_tickets.tickets(id)
        ON DELETE CASCADE,

    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT ticket_updates_content_length
        CHECK (CHAR_LENGTH(BTRIM(content)) BETWEEN 1 AND 500)
);

CREATE INDEX IF NOT EXISTS ticket_updates_ticket_created_idx
    ON secarch_tickets.ticket_updates (ticket_id, created_at DESC);
