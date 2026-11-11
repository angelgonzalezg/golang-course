// Package sqlcgen contains checked-in sqlc output. Edit contactsqlc/queries.sql
// or sqlc.yaml and run go generate from the module root to update the bindings.
//
// DBTX describes the database/sql methods shared by a pool and a transaction.
// Queries binds those methods to named SQL operations; WithTx returns another
// query object using a caller-owned transaction. It does not begin or commit it.
// SqlLabContact is a generated database row, including sql.NullString presence.
// The contactsqlc adapter converts it to the course's public Contact model.
package sqlcgen
