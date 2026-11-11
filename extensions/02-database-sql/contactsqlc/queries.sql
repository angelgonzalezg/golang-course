-- CreateContact inserts one row and returns the database-assigned ID and fields.
-- name: CreateContact :one
INSERT INTO sql_lab_contacts (name, email) VALUES ($1, $2)
RETURNING id, name, email;

-- GetContact returns one contact; a missing row becomes sql.ErrNoRows in Go.
-- name: GetContact :one
SELECT id, name, email FROM sql_lab_contacts WHERE id = $1;

-- ListContacts keeps the manual lesson's explicit ordering and bounded page.
-- name: ListContacts :many
SELECT id, name, email FROM sql_lab_contacts
ORDER BY id LIMIT sqlc.arg(page_size)::integer;

-- UpdateContactEmail assigns email; SQL NULL clears it rather than keeping it.
-- name: UpdateContactEmail :one
UPDATE sql_lab_contacts SET email = $2 WHERE id = $1
RETURNING id, name, email;

-- DeleteContact returns the affected-row count so the adapter can detect absence.
-- name: DeleteContact :execrows
DELETE FROM sql_lab_contacts WHERE id = $1;
