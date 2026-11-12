//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/course/mongo-lab/internal/reportdemo"
	"example.com/course/mongo-lab/reporting"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Exercise lookup, grouping, and explain against actual MongoDB. Each case owns
// a namespace inside this disposable server, separate from manual Compose data.
func TestReferencesAndAggregation(t *testing.T) {
	client, uri := newTestDatabase(t)
	t.Run("reference lifecycle and left lookup", func(t *testing.T) { testReferenceLifecycle(t, client) })
	t.Run("typed reports and counting rules", func(t *testing.T) { testAggregationCounts(t, client) })
	t.Run("explain and canceled readers", func(t *testing.T) { testReportPlans(t, client) })
	t.Run("finite demo cleanup", func(t *testing.T) { testReportDemo(t, client, uri) })
}

// Both collection handles use one database. Only the generated test database
// is dropped, using fresh cleanup time after the operation context is canceled.
func newReportStore(t *testing.T, client *mongo.Client) (*reporting.Store, *mongo.Database) {
	t.Helper()
	database := client.Database("report_test_" + rand.Text())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := database.Drop(ctx); err != nil {
			t.Error(err)
		}
	})
	for _, name := range []string{"profiles", "companies"} {
		if err := database.CreateCollection(operationContext(t), name); err != nil {
			t.Fatal(err)
		}
	}
	store := reporting.NewStore(database, "profiles", "companies")
	if err := store.EnsureIndexes(operationContext(t)); err != nil {
		t.Fatal(err)
	}
	return store, database
}

// Missing references and missing targets have different meanings. The left join
// preserves both, and changing/deleting a company never rewrites source profiles.
func testReferenceLifecycle(t *testing.T, client *mongo.Client) {
	ctx := operationContext(t)
	store, database := newReportStore(t, client)
	company, err := store.CreateCompany(ctx, " Atlas ")
	if err != nil {
		t.Fatal(err)
	}
	ghost := bson.NewObjectID()
	var profiles []reporting.Profile
	for _, input := range []reporting.ProfileInput{
		{Name: "Ana", City: "Guadalajara", CompanyID: &company.ID},
		{Name: "Ana", City: "Guadalajara", CompanyID: &company.ID},
		{Name: "Carla", City: "Guadalajara", CompanyID: &ghost},
		{Name: "Diana", City: "Guadalajara"},
	} {
		created, err := store.CreateProfile(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, created)
	}
	rows, err := store.Directory(ctx, " Guadalajara ", 10)
	if err != nil || len(rows) != 4 {
		t.Fatalf("directory = (%+v, %v)", rows, err)
	}
	if bytes.Compare(rows[0].ID[:], rows[1].ID[:]) >= 0 || rows[0].Company == nil || rows[1].Company == nil || rows[0].Company.Name != "Atlas" {
		t.Fatalf("lookup/sort tie = %+v", rows)
	}
	if rows[2].CompanyID == nil || *rows[2].CompanyID != ghost || rows[2].Company != nil || rows[3].CompanyID != nil || rows[3].Company != nil {
		t.Fatalf("missing-reference semantics = %+v", rows)
	}
	limited, err := store.Directory(ctx, "", 1)
	if err != nil || len(limited) != 1 || limited[0].ID != rows[0].ID {
		t.Fatalf("directory limit = (%+v, %v)", limited, err)
	}
	rows[0].Company.Name = "local edit"
	if rows[1].Company.Name != "Atlas" {
		t.Fatal("decoded companies share mutable storage")
	}
	if _, err := store.RenameCompany(ctx, company.ID, " Atlas Updated "); err != nil {
		t.Fatal(err)
	}
	rows, err = store.Directory(ctx, "", 10)
	if err != nil || rows[0].Company.Name != "Atlas Updated" || rows[1].Company.Name != "Atlas Updated" {
		t.Fatalf("shared rename = (%+v, %v)", rows, err)
	}
	raw, err := database.Collection("profiles").FindOne(ctx, bson.D{{Key: "_id", Value: profiles[0].ID}}).Raw()
	if err != nil || raw.Lookup("company_id").Type != bson.TypeObjectID || raw.Lookup("company_id").ObjectID() != company.ID {
		t.Fatalf("stored reference = (%v, %v)", raw, err)
	}
	if err := store.DeleteCompany(ctx, company.ID); err != nil {
		t.Fatal(err)
	}
	rows, err = store.Directory(ctx, "", 10)
	if err != nil || len(rows) != 4 || rows[0].Company != nil || rows[1].Company != nil || rows[0].CompanyID == nil {
		t.Fatalf("delete did not preserve references = (%+v, %v)", rows, err)
	}
	if err := store.DeleteCompany(ctx, company.ID); !errors.Is(err, reporting.ErrNotFound) {
		t.Fatalf("repeat delete = %v", err)
	}
	if _, err := store.RenameCompany(ctx, ghost, "Unknown"); !errors.Is(err, reporting.ErrNotFound) || !errors.Is(err, mongo.ErrNoDocuments) {
		t.Fatalf("rename missing = %v", err)
	}
}

// Counts use full source groups before limit. Set-union prevents duplicate tags
// in one source from inflating profile counts; empty arrays contribute no tag row.
func testAggregationCounts(t *testing.T, client *mongo.Client) {
	ctx := operationContext(t)
	store, database := newReportStore(t, client)
	cities, err := store.CityTotals(ctx, 10)
	if err != nil || cities == nil || len(cities) != 0 {
		t.Fatalf("empty cities = (%v, %v)", cities, err)
	}
	tags, err := store.TagTotals(ctx, "", 10)
	if err != nil || tags == nil || len(tags) != 0 {
		t.Fatalf("empty tags = (%v, %v)", tags, err)
	}
	var first reporting.Profile
	for i, input := range []reporting.ProfileInput{
		{Name: "Ana", City: "Guadalajara", Tags: []string{"go", "backend"}},
		{Name: "Beto", City: "Guadalajara", Tags: []string{"go"}},
		{Name: "Carla", City: "Monterrey", Tags: []string{"backend"}},
		{Name: "Diana", City: "Monterrey"},
	} {
		profile, err := store.CreateProfile(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = profile
		}
	}
	// Simulate another writer bypassing the Go deduplication rule.
	_, err = database.Collection("profiles").UpdateOne(ctx, bson.D{{Key: "_id", Value: first.ID}}, bson.D{{Key: "$set", Value: bson.D{{Key: "tags", Value: bson.A{"go", "go", "backend"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	cities, err = store.CityTotals(ctx, 10)
	wantCities := []reporting.CityCount{{City: "Guadalajara", Profiles: 2}, {City: "Monterrey", Profiles: 2}}
	if err != nil || !reflect.DeepEqual(cities, wantCities) {
		t.Fatalf("city counts = (%+v, %v)", cities, err)
	}
	cityLimit, err := store.CityTotals(ctx, 1)
	if err != nil || !reflect.DeepEqual(cityLimit, wantCities[:1]) {
		t.Fatalf("group before limit = (%+v, %v)", cityLimit, err)
	}
	tags, err = store.TagTotals(ctx, "", 10)
	wantTags := []reporting.TagCount{{Tag: "backend", Profiles: 2}, {Tag: "go", Profiles: 2}}
	if err != nil || !reflect.DeepEqual(tags, wantTags) {
		t.Fatalf("tag counts = (%+v, %v)", tags, err)
	}
	filtered, err := store.TagTotals(ctx, " Guadalajara ", 1)
	if err != nil || !reflect.DeepEqual(filtered, []reporting.TagCount{{Tag: "go", Profiles: 2}}) {
		t.Fatalf("filtered tag limit = (%+v, %v)", filtered, err)
	}
	if _, err := store.CreateProfile(ctx, reporting.ProfileInput{Name: "Literal", City: `{"$ne":null}`, Tags: []string{"literal"}}); err != nil {
		t.Fatal(err)
	}
	literal, err := store.Directory(ctx, `{"$ne":null}`, 10)
	if err != nil || len(literal) != 1 || literal[0].Name != "Literal" {
		t.Fatalf("literal criterion = (%+v, %v)", literal, err)
	}
	none, err := store.TagTotals(ctx, "guadalajara", 10)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("exact city = (%v, %v)", none, err)
	}
}

// Explain the same stable dataset through three paths. Verify returned sets and
// scan counters, not wall-clock speed or a brittle optimizer-specific plan tree.
func testReportPlans(t *testing.T, client *mongo.Client) {
	ctx := operationContext(t)
	store, database := newReportStore(t, client)
	var documents []any
	for i := range 80 {
		city := "Monterrey"
		if i < 5 {
			city = "Guadalajara"
		}
		documents = append(documents, reporting.Profile{ID: bson.NewObjectID(), Name: fmt.Sprintf("Profile %03d", i), City: city, Tags: []string{}})
	}
	if _, err := database.Collection("profiles").InsertMany(ctx, documents); err != nil {
		t.Fatal(err)
	}
	plans, err := store.ExplainCity(ctx, " Guadalajara ")
	if err != nil {
		t.Fatal(err)
	}
	for _, plan := range []reporting.PlanSummary{plans.Natural, plans.Indexed, plans.Automatic} {
		if plan.Returned != 5 || plan.WinningPlan.Validate() != nil {
			t.Fatalf("explain result = %+v", plan)
		}
	}
	if plans.Natural.DocsExamined != 80 || plans.Natural.KeysExamined != 0 || plans.Indexed.KeysExamined == 0 || plans.Indexed.DocsExamined >= plans.Natural.DocsExamined {
		t.Fatalf("forced path counters: natural=%+v indexed=%+v", plans.Natural, plans.Indexed)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.Directory(canceled, "", 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("directory cancel = %v", err)
	}
	if _, err := store.CityTotals(canceled, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("city cancel = %v", err)
	}
	if _, err := store.TagTotals(canceled, "", 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("tag cancel = %v", err)
	}
	if _, err := store.ExplainCity(canceled, "Guadalajara"); !errors.Is(err, context.Canceled) {
		t.Fatalf("explain cancel = %v", err)
	}
}

// Run the actual finite example twice and ensure cleanup removes only its two
// collections, while a pre-existing company/profile remain in the same database.
func testReportDemo(t *testing.T, client *mongo.Client, uri string) {
	ctx := operationContext(t)
	database := client.Database("mongo_lab")
	kept := reporting.NewStore(database, "kept_report_profiles", "kept_report_companies")
	company, err := kept.CreateCompany(ctx, "Keep company")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := kept.CreateProfile(ctx, reporting.ProfileInput{Name: "Keep profile", City: "Puebla", CompanyID: &company.ID})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var output bytes.Buffer
		if err := reportdemo.Run(ctx, uri, &output); err != nil {
			t.Fatal(err)
		}
		for _, summary := range []string{"Directory keeps five profiles", "does not cascade", "City Guadalajara -> 3", "Tag backend -> 3", "Automatic winning plan"} {
			if !strings.Contains(output.String(), summary) {
				t.Errorf("demo missing %q", summary)
			}
		}
		rows, err := kept.Directory(ctx, "", 10)
		if err != nil || len(rows) != 1 || rows[0].ID != profile.ID || rows[0].Company == nil || rows[0].Company.ID != company.ID {
			t.Fatalf("neighbor cleanup = (%+v, %v)", rows, err)
		}
		names, err := database.ListCollectionNames(ctx, bson.D{{Key: "name", Value: bson.D{{Key: "$regex", Value: "^report_demo_"}}}})
		if err != nil || len(names) != 0 {
			t.Fatalf("demo collections remain = (%v, %v)", names, err)
		}
	}
}
