package contacts

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func queryStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore()
	for _, name := range []string{"Zoë", "Ana", "ana", "Bob", "Álvaro"} {
		if _, err := store.Create(t.Context(), Input{Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestSearchFiltersOrdersThenPaginates(t *testing.T) {
	store := queryStore(t)
	for _, tc := range []struct {
		name  string
		query Query
		ids   []int64
		total int
	}{
		{"default order", Query{Limit: 20}, []int64{1, 2, 3, 4, 5}, 5},
		{"descending IDs", Query{Limit: 20, Sort: SortIDDesc}, []int64{5, 4, 3, 2, 1}, 5},
		{"names with tie", Query{Limit: 20, Sort: SortName}, []int64{2, 3, 4, 1, 5}, 5},
		{"descending names with tie", Query{Limit: 20, Sort: SortNameDesc}, []int64{5, 1, 4, 2, 3}, 5},
		{"first page", Query{Limit: 2}, []int64{1, 2}, 5},
		{"last short page", Query{Limit: 2, Offset: 4}, []int64{5}, 5},
		{"filter before offset", Query{Limit: 1, Offset: 1, NameContains: " AN ", Sort: SortNameDesc}, []int64{3}, 2},
		{"Unicode lowercase", Query{Limit: 20, NameContains: "Ë"}, []int64{1}, 1},
		{"accent retained", Query{Limit: 20, NameContains: "alv"}, []int64{}, 0},
		{"offset at end", Query{Limit: 2, Offset: 5}, []int64{}, 5},
		{"large offset", Query{Limit: 2, Offset: math.MaxInt}, []int64{}, 5},
		{"maximum limit", Query{Limit: MaxLimit}, []int64{1, 2, 3, 4, 5}, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := store.Search(t.Context(), tc.query)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]int64, 0, len(page.Contacts))
			for _, contact := range page.Contacts {
				ids = append(ids, contact.ID)
			}
			if page.Contacts == nil || page.Total != tc.total || !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("page = %+v, IDs %v; want total %d, IDs %v", page, ids, tc.total, tc.ids)
			}
		})
	}
}

func TestSearchNormalizationAndSnapshot(t *testing.T) {
	store := queryStore(t)
	page, err := store.Search(t.Context(), Query{Limit: 2, NameContains: " AN "})
	if err != nil {
		t.Fatal(err)
	}
	if page.Query != (Query{Limit: 2, NameContains: "AN", Sort: SortID}) {
		t.Fatalf("normalized query = %+v", page.Query)
	}
	page.Contacts[0].Name = "Edited snapshot"
	stored, err := store.Get(t.Context(), 2)
	if err != nil || stored.Name != "Ana" {
		t.Fatalf("editing page changed store: %+v, %v", stored, err)
	}
	if err := store.Delete(t.Context(), 3); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Contacts[1].Name != "ana" {
		t.Fatalf("later store mutation changed an earlier page: %+v", page)
	}
	newPage, err := store.Search(t.Context(), Query{Limit: 2, NameContains: "AN"})
	if err != nil || newPage.Total != 1 {
		t.Fatalf("next request must use current state: %+v, %v", newPage, err)
	}
}

func TestSearchRejectsInvalidQueries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query Query
		field string
	}{
		{"missing limit", Query{}, "limit"},
		{"negative limit", Query{Limit: -1}, "limit"},
		{"excessive limit", Query{Limit: 101}, "limit"},
		{"negative offset", Query{Limit: 20, Offset: -1}, "offset"},
		{"unsupported sort", Query{Limit: 20, Sort: "email"}, "sort"},
		{"long filter", Query{Limit: 20, NameContains: strings.Repeat("界", 81)}, "name_contains"},
		{"invalid UTF-8", Query{Limit: 20, NameContains: "\xff"}, "name_contains"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewStore().Search(t.Context(), tc.query)
			var validation *ValidationError
			if !errors.Is(err, ErrInvalidInput) || !errors.As(err, &validation) || validation.Field != tc.field {
				t.Fatalf("query error = %v; want validation of %s", err, tc.field)
			}
		})
	}
	page, err := NewStore().Search(t.Context(), Query{Limit: 20})
	if err != nil || page.Contacts == nil || len(page.Contacts) != 0 || page.Total != 0 {
		t.Fatalf("empty store page = %+v, %v", page, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewStore().Search(ctx, Query{Limit: 20}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled search = %v", err)
	}
}

func TestSearchUsesConsistentSnapshotsDuringWrites(t *testing.T) {
	store := NewStore()
	contact, err := store.Create(t.Context(), Input{Name: "Chosen"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Go(func() {
		<-start
		for range 40 {
			for _, name := range []string{"Other", "Chosen"} {
				if _, err := store.Patch(t.Context(), contact.ID, Changes{Name: text(name)}); err != nil {
					t.Error(err)
				}
			}
		}
	})
	for range 3 {
		wg.Go(func() {
			<-start
			for range 40 {
				page, err := store.Search(t.Context(), Query{Limit: 20, NameContains: "chosen", Sort: SortName})
				if err != nil {
					t.Error(err)
					return
				}
				if page.Total != len(page.Contacts) || page.Total > 1 {
					t.Errorf("count and contacts disagree: %+v", page)
				}
				for _, got := range page.Contacts {
					if got.Name != "Chosen" {
						t.Errorf("page contains a nonmatching contact: %+v", got)
					}
				}
			}
		})
	}
	close(start)
	wg.Wait()
}
