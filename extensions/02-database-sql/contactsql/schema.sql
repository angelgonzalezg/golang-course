-- This bootstrap creates a new teaching table; it does not migrate an old schema.
CREATE TABLE IF NOT EXISTS sql_lab_contacts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL,
    email text,
    CONSTRAINT sql_lab_contacts_name_length CHECK (char_length(name) BETWEEN 1 AND 80),
    CONSTRAINT sql_lab_contacts_name_key UNIQUE (name),
    CONSTRAINT sql_lab_contacts_email_length CHECK (char_length(email) <= 254)
);
