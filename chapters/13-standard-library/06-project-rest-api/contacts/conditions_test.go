package contacts

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func matching(contact Contact) Conditions {
	return Conditions{Match: &VersionMatch{Versions: []string{Version(contact)}}}
}

func TestConditionalMutationsRejectStaleVersions(t *testing.T) {
	store := NewStore()
	before, err := store.Create(t.Context(), Input{Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.ReplaceIf(t.Context(), before.ID, Input{Name: "Ana Torres"}, matching(before))
	if err != nil || updated.Name != "Ana Torres" || Version(updated) == Version(before) {
		t.Fatalf("conditional replacement = %+v, %v", updated, err)
	}
	_, replaceErr := store.ReplaceIf(t.Context(), before.ID, Input{Name: "Stale replacement"}, matching(before))
	_, patchErr := store.PatchIf(t.Context(), before.ID, Changes{Email: text("stale@example.com")}, matching(before))
	for _, err := range []error{replaceErr, patchErr, store.DeleteIf(t.Context(), before.ID, matching(before))} {
		if !errors.Is(err, ErrPreconditionFailed) {
			t.Errorf("stale mutation = %v", err)
		}
	}
	got, err := store.Get(t.Context(), before.ID)
	if err != nil || got != updated {
		t.Fatalf("stale operations changed contact: %+v, %v", got, err)
	}
	if _, err := store.Create(t.Context(), Input{Name: "Ana Torres"}); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("name index changed after rejection: %v", err)
	}
	if err := store.DeleteIf(t.Context(), updated.ID, matching(updated)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), updated.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("conditional deletion did not remove contact: %v", err)
	}
}

func TestVersionConditionsAndNormalValidation(t *testing.T) {
	store := NewStore()
	contact, err := store.Create(t.Context(), Input{Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	version := Version(contact)
	for _, tc := range []struct {
		name       string
		conditions Conditions
		fail       bool
	}{
		{"unconditional", Conditions{}, false},
		{"any existing", Conditions{Match: &VersionMatch{Any: true}}, false},
		{"one matching alternative", Conditions{Match: &VersionMatch{Versions: []string{"other", version}}}, false},
		{"empty match list", Conditions{Match: &VersionMatch{}}, true},
		{"none of another version", Conditions{NoneMatch: &VersionMatch{Versions: []string{"other"}}}, false},
		{"none of current", Conditions{NoneMatch: &VersionMatch{Versions: []string{version}}}, true},
		{"none of any existing", Conditions{NoneMatch: &VersionMatch{Any: true}}, true},
		{"empty none-match list", Conditions{NoneMatch: &VersionMatch{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.PatchIf(t.Context(), contact.ID, Changes{}, tc.conditions)
			if tc.fail {
				if !errors.Is(err, ErrPreconditionFailed) {
					t.Fatalf("condition = %v", err)
				}
			} else if err != nil || got != contact || Version(got) != version {
				t.Fatalf("no-op must preserve content and version: %+v, %v", got, err)
			}
		})
	}
	stale := Conditions{Match: &VersionMatch{Versions: []string{"stale"}}}
	if _, err := store.PatchIf(t.Context(), contact.ID, Changes{Name: text("")}, stale); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("normal validation must precede conditions: %v", err)
	}
	if _, err := store.PatchIf(t.Context(), 999, Changes{}, stale); !errors.Is(err, ErrNotFound) {
		t.Fatalf("normal missing-resource response = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.DeleteIf(ctx, contact.ID, matching(contact)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled conditional deletion = %v", err)
	}
	// Versions cover field boundaries and identity, not just concatenated text.
	if Version(Contact{ID: 1, Name: "ab", Email: "c"}) == Version(Contact{ID: 1, Name: "a", Email: "bc"}) ||
		Version(contact) == Version(Contact{ID: contact.ID + 1, Name: contact.Name}) {
		t.Fatal("version omitted a field boundary or ID")
	}
}

func TestOnlyOneWriterCanUseTheSameVersion(t *testing.T) {
	store := NewStore()
	before, err := store.Create(t.Context(), Input{Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"Writer A", "Writer B"} {
		wg.Go(func() {
			<-start
			_, err := store.PatchIf(t.Context(), before.ID, Changes{Name: text(name)}, matching(before))
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	success, rejected := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrPreconditionFailed) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("writers: successful %d, rejected %d; want one of each", success, rejected)
	}
}
