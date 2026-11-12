package reporting

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// PlanSummary retains counters plus the server's winning-plan document. Plan
// shapes can change across engines/versions; do not hardcode one tree layout.
type PlanSummary struct {
	Returned     int64
	DocsExamined int64
	KeysExamined int64
	WinningPlan  bson.Raw
}

type PlanComparison struct {
	Natural   PlanSummary
	Indexed   PlanSummary
	Automatic PlanSummary
}

// ExplainCity compares the same find/filter/sort with a forced natural scan,
// a forced city index, and no hint. EnsureIndexes must run first. This explains
// the directory's local selection/order, not the entire lookup/group pipelines.
func (s *Store) ExplainCity(ctx context.Context, city string) (PlanComparison, error) {
	city, err := text("city", city, 80)
	if err != nil {
		return PlanComparison{}, err
	}
	natural, err := s.explain(ctx, city, bson.D{{Key: "$natural", Value: 1}})
	if err != nil {
		return PlanComparison{}, err
	}
	indexed, err := s.explain(ctx, city, cityIndex)
	if err != nil {
		return PlanComparison{}, err
	}
	automatic, err := s.explain(ctx, city, nil)
	if err != nil {
		return PlanComparison{}, err
	}
	return PlanComparison{Natural: natural, Indexed: indexed, Automatic: automatic}, nil
}

func (s *Store) explain(ctx context.Context, city string, hint any) (PlanSummary, error) {
	find := bson.D{
		{Key: "find", Value: s.profiles.Name()}, {Key: "filter", Value: bson.D{{Key: "city", Value: city}}},
		{Key: "sort", Value: bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}},
	}
	if hint != nil {
		find = append(find, bson.E{Key: "hint", Value: hint})
	}
	var reply struct {
		QueryPlanner struct {
			WinningPlan bson.Raw `bson:"winningPlan"`
		} `bson:"queryPlanner"`
		ExecutionStats struct {
			Returned int64 `bson:"nReturned"`
			Docs     int64 `bson:"totalDocsExamined"`
			Keys     int64 `bson:"totalKeysExamined"`
		} `bson:"executionStats"`
	}
	err := s.profiles.Database().RunCommand(ctx, bson.D{
		{Key: "explain", Value: find}, {Key: "verbosity", Value: "executionStats"},
	}).Decode(&reply)
	if err != nil {
		return PlanSummary{}, fmt.Errorf("explain city query: %w", queryError(ctx, err))
	}
	return PlanSummary{Returned: reply.ExecutionStats.Returned, DocsExamined: reply.ExecutionStats.Docs,
		KeysExamined: reply.ExecutionStats.Keys, WinningPlan: reply.QueryPlanner.WinningPlan}, nil
}
