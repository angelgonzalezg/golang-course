BEGIN;
DROP TABLE sql_tx_contact_changes;
ALTER TABLE sql_tx_contacts DROP COLUMN version;
COMMIT;
