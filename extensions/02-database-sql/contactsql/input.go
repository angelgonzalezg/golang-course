package contactsql

// NormalizeInput applies the same text rules used by Create. The transaction,
// idempotency, and sqlc lessons reuse these rules for their storage operations.
// The optional email is borrowed; keep it stable until the operation returns.
func NormalizeInput(input Input) (Input, error) { return normalize(input) }
