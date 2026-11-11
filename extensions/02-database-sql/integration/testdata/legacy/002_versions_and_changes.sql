ALTER TABLE sql_tx_contacts
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

CREATE TABLE sql_tx_contact_changes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    contact_id bigint NOT NULL REFERENCES sql_tx_contacts(id) ON DELETE CASCADE,
    from_version bigint NOT NULL CHECK (from_version > 0),
    to_version bigint NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT current_timestamp,
    CONSTRAINT sql_tx_contact_changes_step CHECK (to_version = from_version + 1),
    CONSTRAINT sql_tx_contact_changes_once UNIQUE (contact_id, to_version)
);
