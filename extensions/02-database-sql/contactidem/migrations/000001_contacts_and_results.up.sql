BEGIN;

CREATE TABLE sql_idem_contacts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    email text CHECK (char_length(email) <= 254),
    CONSTRAINT sql_idem_contacts_name_key UNIQUE (name)
);

-- A row is a complete successful result. There is no committed pending state:
-- the contact and this record are inserted in one transaction.
CREATE TABLE sql_idem_create_requests (
    subject text NOT NULL CHECK (subject ~ '^[A-Za-z0-9_-]{1,64}$'),
    request_key text NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{1,64}$'),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    response_id bigint NOT NULL CHECK (response_id > 0),
    response_name text NOT NULL CHECK (char_length(response_name) BETWEEN 1 AND 80),
    response_email text CHECK (char_length(response_email) <= 254),
    created_at timestamptz NOT NULL DEFAULT current_timestamp,
    PRIMARY KEY (subject, request_key)
);

-- Deliberately no foreign key from response_id to the live contact. Deleting
-- a contact must not erase its saved result or let an old key create it again.

COMMIT;
