BEGIN;
-- Dropping the result table forgets its keys and their replay guarantees.
DROP TABLE sql_service_create_requests;
DROP TABLE sql_service_contacts;
COMMIT;
