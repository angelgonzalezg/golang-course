// Package reportdemo runs finite references, aggregations, and explain examples.
package reportdemo

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"example.com/course/mongo-lab/contactmongo"
	"example.com/course/mongo-lab/reporting"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Run owns its client and two random collections. Cleanup drops only those
// collections with fresh budgets, then disconnects after their users finish.
func Run(ctx context.Context, uri string, output io.Writer) (result error) {
	client, err := contactmongo.Open(ctx, uri)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		result = errors.Join(result, client.Disconnect(cleanup))
	}()
	database := client.Database("mongo_lab")
	prefix := "report_demo_" + strings.ToLower(rand.Text()[:12])
	profilesName, companiesName := prefix+"_profiles", prefix+"_companies"
	for _, name := range []string{profilesName, companiesName} {
		if err := database.CreateCollection(ctx, name); err != nil {
			return err
		}
		// Register each collection immediately, including if later creation fails.
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result = errors.Join(result, database.Collection(name).Drop(cleanup))
		}()
	}
	store := reporting.NewStore(database, profilesName, companiesName)
	if err := store.EnsureIndexes(ctx); err != nil {
		return err
	}
	fmt.Fprintf(output, "Owned demo collections: %s, %s\n", profilesName, companiesName)
	fmt.Fprintln(output, "\n== 1. Store companies and ObjectID references ==")
	atlas, err := store.CreateCompany(ctx, "Atlas")
	if err != nil {
		return err
	}
	orbit, err := store.CreateCompany(ctx, "Orbit")
	if err != nil {
		return err
	}
	dangling := bson.NewObjectID()
	for _, input := range []reporting.ProfileInput{
		{Name: "Ana", City: "Guadalajara", Tags: []string{"go", "backend"}, CompanyID: &atlas.ID},
		{Name: "Beto", City: "Guadalajara", Tags: []string{"go"}, CompanyID: &atlas.ID},
		{Name: "Carla", City: "Monterrey", Tags: []string{"backend"}, CompanyID: &orbit.ID},
		{Name: "Diana", City: "Guadalajara"},
		{Name: "Eva", City: "Puebla", Tags: []string{"go", "backend"}, CompanyID: &dangling},
	} {
		if _, err := store.CreateProfile(ctx, input); err != nil {
			return err
		}
	}
	fmt.Fprintln(output, "Stored five profiles: three valid references, one unassigned, one dangling.")

	fmt.Fprintln(output, "\n== 2. Left lookup and typed directory results ==")
	rows, err := store.Directory(ctx, "", 10)
	if err != nil {
		return err
	}
	if len(rows) != 5 || rows[3].CompanyID != nil || rows[3].Company != nil || rows[4].CompanyID == nil || rows[4].Company != nil {
		return errors.New("left lookup did not preserve unassigned/dangling profiles")
	}
	for _, row := range rows {
		company := "unassigned"
		if row.Company != nil {
			company = row.Company.Name
		} else if row.CompanyID != nil {
			company = "missing company"
		}
		fmt.Fprintf(output, "%s (%s) -> %s\n", row.Name, row.City, company)
	}
	fmt.Fprintln(output, "Directory keeps five profiles; missing targets do not remove their source rows.")

	fmt.Fprintln(output, "\n== 3. Shared company changes and reference lifetime ==")
	if _, err := store.RenameCompany(ctx, atlas.ID, "Atlas Updated"); err != nil {
		return err
	}
	rows, err = store.Directory(ctx, "Guadalajara", 10)
	if err != nil {
		return err
	}
	if len(rows) != 3 || rows[0].Company == nil || rows[1].Company == nil || rows[0].Company.Name != "Atlas Updated" || rows[1].Company.Name != "Atlas Updated" {
		return errors.New("referencing profiles did not see the renamed company")
	}
	if err := store.DeleteCompany(ctx, orbit.ID); err != nil {
		return err
	}
	rows, err = store.Directory(ctx, "Monterrey", 10)
	if err != nil {
		return err
	}
	if len(rows) != 1 || rows[0].CompanyID == nil || rows[0].Company != nil {
		return errors.New("company deletion unexpectedly removed its profile")
	}
	fmt.Fprintln(output, "One company rename appears in two profiles; deleting another company does not cascade.")

	fmt.Fprintln(output, "\n== 4. Group complete input before limiting reports ==")
	cities, err := store.CityTotals(ctx, 10)
	if err != nil {
		return err
	}
	if len(cities) != 3 || cities[0].City != "Guadalajara" || cities[0].Profiles != 3 {
		return errors.New("unexpected city totals")
	}
	for _, row := range cities {
		fmt.Fprintf(output, "City %s -> %d profiles\n", row.City, row.Profiles)
	}
	tags, err := store.TagTotals(ctx, "", 10)
	if err != nil {
		return err
	}
	if len(tags) != 2 || tags[0].Tag != "backend" || tags[0].Profiles != 3 || tags[1].Tag != "go" || tags[1].Profiles != 3 {
		return errors.New("unexpected tag totals")
	}
	for _, row := range tags {
		fmt.Fprintf(output, "Tag %s -> %d profiles\n", row.Tag, row.Profiles)
	}
	fmt.Fprintln(output, "Empty tags add no tag rows; a profile can contribute to several tag groups.")

	fmt.Fprintln(output, "\n== 5. Explain natural, indexed, and automatic access ==")
	plans, err := store.ExplainCity(ctx, "Guadalajara")
	if err != nil {
		return err
	}
	for _, item := range []struct {
		label string
		plan  reporting.PlanSummary
	}{
		{"Natural", plans.Natural}, {"City index", plans.Indexed}, {"Automatic", plans.Automatic},
	} {
		if item.plan.Returned != 3 {
			return errors.New("explain paths returned different city results")
		}
		fmt.Fprintf(output, "%s: returned=%d, documents examined=%d, keys examined=%d\n", item.label, item.plan.Returned, item.plan.DocsExamined, item.plan.KeysExamined)
	}
	fmt.Fprintf(output, "Automatic winning plan: %s\n", plans.Automatic.WinningPlan.String())
	fmt.Fprintln(output, "Hints compare access paths; counters and optimizer plans depend on the workload.")
	fmt.Fprintln(output, "Cleanup drops only the two owned demo collections.")
	return nil
}
