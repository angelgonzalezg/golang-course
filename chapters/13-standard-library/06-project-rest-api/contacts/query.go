package contacts

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

type SortOrder string

const (
	SortID       SortOrder = "id"
	SortIDDesc   SortOrder = "-id"
	SortName     SortOrder = "name"
	SortNameDesc SortOrder = "-name"
)

// Query selects one page without depending on URL or HTTP types. Callers must
// supply a limit in 1..MaxLimit; transport adapters apply DefaultLimit themselves.
// An empty sort selects increasing ID order. Limit bounds output, not search work.
type Query struct {
	Limit        int
	Offset       int
	NameContains string
	Sort         SortOrder
}

// Page owns its contact slice. Total counts all matches before pagination.
// Query contains normalized values so a transport can describe the exact page
// and build navigation links.
type Page struct {
	Contacts []Contact
	Total    int
	Query    Query
}

// NormalizeQuery validates paging/filter options for every storage backend.
func NormalizeQuery(query Query) (Query, error) {
	if query.Limit < 1 || query.Limit > MaxLimit {
		return Query{}, &ValidationError{Field: "limit", Message: fmt.Sprintf("must be between 1 and %d", MaxLimit)}
	}
	if query.Offset < 0 {
		return Query{}, &ValidationError{Field: "offset", Message: "must not be negative"}
	}
	query.NameContains = strings.TrimSpace(query.NameContains)
	if !utf8.ValidString(query.NameContains) || utf8.RuneCountInString(query.NameContains) > 80 {
		return Query{}, &ValidationError{Field: "name_contains", Message: "must contain at most 80 valid UTF-8 runes"}
	}
	if query.Sort == "" {
		query.Sort = SortID
	}
	switch query.Sort {
	case SortID, SortIDDesc, SortName, SortNameDesc:
	default:
		return Query{}, &ValidationError{Field: "sort", Message: "must be id, -id, name, or -name"}
	}
	return query, nil
}

// Search filters and orders a snapshot, then selects one page. A later request
// takes a new snapshot: offsets do not freeze the collection between requests.
func (s *Store) Search(ctx context.Context, query Query) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	query, err := NormalizeQuery(query)
	if err != nil {
		return Page{}, err
	}
	values, err := s.List(ctx)
	if err != nil {
		return Page{}, err
	}
	return SelectPage(ctx, values, query)
}

// SelectPage takes ownership of a private snapshot in increasing ID order.
// Both stores use Go's Unicode lowercase, name comparison, and ID tie-breaker,
// so database collation cannot change collection responses or their ETags.
// The returned page owns its slice and does not retain the full snapshot.
func SelectPage(ctx context.Context, values []Contact, query Query) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	query, err := NormalizeQuery(query)
	if err != nil {
		return Page{}, err
	}
	needle := strings.ToLower(query.NameContains)
	// The caller supplied a private slice. Filtering it cannot edit the store.
	matched := values[:0]
	for _, contact := range values {
		if err := ctx.Err(); err != nil {
			return Page{}, err
		}
		if strings.Contains(strings.ToLower(contact.Name), needle) {
			matched = append(matched, contact)
		}
	}
	switch query.Sort {
	case SortIDDesc:
		slices.Reverse(matched)
	case SortName, SortNameDesc:
		slices.SortFunc(matched, func(a, b Contact) int {
			order := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			if order == 0 {
				// Ana and ana are distinct contacts but have the same sort key.
				// An ID tie-breaker keeps their order predictable on every page.
				return cmp.Compare(a.ID, b.ID)
			}
			if query.Sort == SortNameDesc {
				return -order
			}
			return order
		})
	}
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	start := min(query.Offset, len(matched))
	// Subtract before adding to avoid overflowing a large requested offset.
	end := start + min(query.Limit, len(matched)-start)
	page := make([]Contact, end-start)
	copy(page, matched[start:end]) // The result does not retain the whole snapshot.
	return Page{Contacts: page, Total: len(matched), Query: query}, nil
}
