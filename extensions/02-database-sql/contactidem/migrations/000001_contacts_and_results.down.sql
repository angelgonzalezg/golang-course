BEGIN;
-- Removing request records also removes the corresponding replay guarantees.
DROP TABLE sql_idem_create_requests;
DROP TABLE sql_idem_contacts;
COMMIT;
