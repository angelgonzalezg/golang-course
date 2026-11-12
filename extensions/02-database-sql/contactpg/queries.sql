-- CreateContact stores normalized domain values and returns their assigned ID.
-- name: CreateContact :one
INSERT INTO sql_service_contacts (name, email, phone) VALUES ($1, $2, $3)
RETURNING id, name, email, phone;

-- GetContact reads the current resource rather than an earlier creation result.
-- name: GetContact :one
SELECT id, name, email, phone FROM sql_service_contacts WHERE id = $1;

-- LockContact holds the row until the caller's transaction commits/rolls back.
-- name: LockContact :one
SELECT id, name, email, phone FROM sql_service_contacts WHERE id = $1 FOR UPDATE;

-- ListContacts supplies one consistent, ID-ordered snapshot to shared Go paging.
-- name: ListContacts :many
SELECT id, name, email, phone FROM sql_service_contacts ORDER BY id;

-- NameTaken checks the usual validation priority before version conditions.
-- The unique constraint still handles a concurrent writer claiming this name.
-- name: NameTaken :one
SELECT EXISTS (SELECT 1 FROM sql_service_contacts WHERE name = $1 AND id <> $2);

-- ReplaceContact writes all editable fields through the caller-owned transaction.
-- name: ReplaceContact :one
UPDATE sql_service_contacts SET name = $2, email = $3, phone = $4 WHERE id = $1
RETURNING id, name, email, phone;

-- DeleteContact reports whether the locked resource was removed.
-- name: DeleteContact :execrows
DELETE FROM sql_service_contacts WHERE id = $1;

-- LockCreation serializes one caller/key across connections and processes.
-- name: LockCreation :exec
SELECT pg_advisory_xact_lock(sqlc.arg(lock_id)::bigint);

-- CreationResult is read in a fresh statement AFTER the key lock is acquired.
-- name: CreationResult :one
SELECT request_hash, response_id, response_name, response_email, response_phone
FROM sql_service_create_requests WHERE subject = $1 AND request_key = $2;

-- SaveCreationResult shares the contact insert's transaction; no partial
-- successful creation or committed pending reservation is exposed.
-- name: SaveCreationResult :exec
INSERT INTO sql_service_create_requests
    (subject, request_key, request_hash, response_id, response_name, response_email, response_phone)
VALUES ($1, $2, $3, $4, $5, $6, $7);
