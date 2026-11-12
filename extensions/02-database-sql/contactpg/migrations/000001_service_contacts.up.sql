BEGIN;

-- The shared Go model permits every valid UTF-8 string, including NUL. bytea
-- preserves those bytes; PostgreSQL text cannot contain NUL. Application rules
-- check rune lengths; these byte bounds provide an additional storage limit.
CREATE TABLE sql_service_contacts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name bytea NOT NULL CHECK (octet_length(name) BETWEEN 1 AND 320),
    email bytea NOT NULL CHECK (octet_length(email) <= 1016),
    phone bytea NOT NULL CHECK (octet_length(phone) <= 160),
    CONSTRAINT sql_service_contacts_name_key UNIQUE (name)
);

-- Save a historical result, with no foreign key to the current contact. A
-- later edit/delete must not alter replay or let an old request recreate data.
CREATE TABLE sql_service_create_requests (
    subject text NOT NULL CHECK (subject ~ '^[A-Za-z0-9_-]{1,64}$'),
    request_key text NOT NULL CHECK (request_key ~ '^[A-Za-z0-9_-]{1,64}$'),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    response_id bigint NOT NULL CHECK (response_id > 0),
    response_name bytea NOT NULL,
    response_email bytea NOT NULL,
    response_phone bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT current_timestamp,
    PRIMARY KEY (subject, request_key)
);

COMMIT;
