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

// Query describes a bounded search without depending on URL or HTTP types.
// Callers supply a limit; an empty sort selects increasing ID order.
type Query struct {
	Limit        int
	Offset       int
	NameContains string
	Sort         SortOrder
}

// Total counts all matches before pagination. Query contains normalized values
// so a transport can describe the exact page and build navigation links.
type Page struct {
	Contacts []Contact
	Total    int
	Query    Query
}

func normalizeQuery(query Query) (Query, error) {
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
	query, err := normalizeQuery(query)
	if err != nil {
		return Page{}, err
	}
	values, err := s.List(ctx)
	if err != nil {
		return Page{}, err
	}
	needle := strings.ToLower(query.NameContains)
	// List returned our own slice. Filtering it in place cannot edit the store.
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
