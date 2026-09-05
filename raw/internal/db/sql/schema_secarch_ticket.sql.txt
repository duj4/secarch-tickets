-- sql/schema_secarch_ticket.sql

CREATE TABLE IF NOT EXISTS secarch_tickets.tickets (
    id BIGSERIAL PRIMARY KEY,

    ticket_number TEXT NOT NULL UNIQUE,

    summary          TEXT NOT NULL,
    reporter         TEXT NOT NULL,
    assignee         TEXT,
    cmdb_system_name TEXT[],
    cmdb_system_key  TEXT NOT NULL DEFAULT '',
    department       TEXT NOT NULL DEFAULT '',

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

ALTER TABLE secarch_tickets.tickets
    ADD COLUMN IF NOT EXISTS cmdb_system_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS department TEXT NOT NULL DEFAULT '';

UPDATE secarch_tickets.tickets
SET cmdb_system_key = COALESCE(cmdb_system_key, ''),
    department = COALESCE(department, '')
WHERE cmdb_system_key IS NULL
   OR department IS NULL;

ALTER TABLE secarch_tickets.tickets
    ALTER COLUMN cmdb_system_key SET DEFAULT '',
    ALTER COLUMN cmdb_system_key SET NOT NULL,
    ALTER COLUMN department SET DEFAULT '',
    ALTER COLUMN department SET NOT NULL;

CREATE INDEX IF NOT EXISTS tickets_open_idx
    ON secarch_tickets.tickets (ticket_number)
    WHERE ticket_closed_at IS NULL;

CREATE INDEX IF NOT EXISTS tickets_closed_at_idx
    ON secarch_tickets.tickets (ticket_closed_at)
    WHERE ticket_closed_at IS NOT NULL;
