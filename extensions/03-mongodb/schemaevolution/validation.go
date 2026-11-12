// Package schemaevolution teaches a forward rollout of MongoDB validation and
// a repeatable data backfill. Collection policy and document versions are separate.
package schemaevolution

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Store the rules as readable JSON so the validator can be studied beside Go.
// Unknown fields are allowed; migrating a known field must not erase other data.
//
//go:embed schemas/v1.json
var legacyJSON []byte

//go:embed schemas/v2.json
var currentJSON []byte

var (
	ErrInvalidInput       = errors.New("invalid schema lesson input")
	ErrInvalidDocument    = errors.New("document does not match a supported schema")
	ErrUnsupportedVersion = errors.New("unsupported document schema version")
	ErrNotFound           = errors.New("profile not found")
	ErrMigrationRequired  = errors.New("documents still require migration or repair")
)

// InstallCompatible is the explicit expansion step: strict/error validation
// accepts either valid v1 or valid v2. It does not inspect or repair old data.
// Run this during the planned expansion, not automatically on every startup:
// applying it after Finalize would intentionally reopen writes using v1.
func InstallCompatible(ctx context.Context, schemaDatabase *mongo.Database, collection string) error {
	validator, err := compatibleValidator()
	if err != nil {
		return err
	}
	return install(ctx, schemaDatabase, collection, validator)
}

// CheckCompatible audits existing documents, including rows that predate the
// validator. BackfillBatch only transforms valid v1; fix malformed/future data
// explicitly instead of silently coercing values or assigning guessed versions.
func CheckCompatible(ctx context.Context, collection *mongo.Collection) error {
	validator, err := compatibleValidator()
	if err != nil {
		return err
	}
	count, err := collection.CountDocuments(ctx, bson.D{{Key: "$nor", Value: bson.A{validator}}})
	if err != nil {
		return fmt.Errorf("audit compatible documents: %w", queryError(ctx, err))
	}
	if count != 0 {
		return fmt.Errorf("%d incompatible document(s): %w", count, ErrInvalidDocument)
	}
	return nil
}

// Finalize closes old writes BEFORE auditing all data against v2. MongoDB 8's
// strict validator checks writes, not existing rows retroactively. A failed audit
// leaves the v2 policy installed: rerun the backfill/repair, then audit again.
// Retire old writers before this cutover; this is not a multi-document transaction.
func Finalize(ctx context.Context, schemaDatabase *mongo.Database, collection *mongo.Collection) error {
	if schemaDatabase.Name() != collection.Database().Name() {
		return fmt.Errorf("schema and data database names must match: %w", ErrInvalidInput)
	}
	validator, err := decodeValidator(currentJSON)
	if err != nil {
		return err
	}
	if err := install(ctx, schemaDatabase, collection.Name(), validator); err != nil {
		return err
	}
	count, err := collection.CountDocuments(ctx, bson.D{{Key: "$nor", Value: bson.A{validator}}})
	if err != nil {
		return fmt.Errorf("audit current documents: %w", queryError(ctx, err))
	}
	if count != 0 {
		return fmt.Errorf("v2 policy installed; %d document(s) remain: %w", count, ErrMigrationRequired)
	}
	return nil
}

// install uses a schema-administration connection. The normal readWrite role
// cannot run collMod; validationAction=error rejects bad writes with code 121.
func install(ctx context.Context, database *mongo.Database, collection string, validator bson.D) error {
	if collection == "" {
		return fmt.Errorf("collection name is required: %w", ErrInvalidInput)
	}
	err := database.RunCommand(ctx, bson.D{
		{Key: "collMod", Value: collection},
		{Key: "validator", Value: validator},
		{Key: "validationLevel", Value: "strict"},
		{Key: "validationAction", Value: "error"},
	}).Err()
	if err != nil {
		return fmt.Errorf("install collection validator: %w", queryError(ctx, err))
	}
	return nil
}

func compatibleValidator() (bson.D, error) {
	legacy, err := decodeValidator(legacyJSON)
	if err != nil {
		return nil, err
	}
	current, err := decodeValidator(currentJSON)
	if err != nil {
		return nil, err
	}
	return bson.D{{Key: "$or", Value: bson.A{legacy, current}}}, nil
}

func decodeValidator(data []byte) (bson.D, error) {
	var validator bson.D
	if err := bson.UnmarshalExtJSON(data, false, &validator); err != nil {
		return nil, fmt.Errorf("decode embedded validator: %w", err)
	}
	return validator, nil
}

// IsValidationFailure recognizes a stable server code through wrapped errors.
// Inspect the driver error for details; never classify by localized message text.
func IsValidationFailure(err error) bool {
	var serverError mongo.ServerError
	return errors.As(err, &serverError) && serverError.HasErrorCode(121)
}

func queryError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}
