package contactsql

// NormalizeInput applies the same text rules used by Create. The transaction
// lesson reuses these rules while storing its own versioned contact model.
// The optional email is borrowed; keep it stable until the operation returns.
func NormalizeInput(input Input) (Input, error) { return normalize(input) }
