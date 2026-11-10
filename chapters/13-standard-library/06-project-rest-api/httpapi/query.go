package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

func (a *api) readQuery(w http.ResponseWriter, r *http.Request) (contacts.Query, bool) {
	// URL.Query discards parsing errors. ParseQuery lets us reject malformed
	// escapes and unescaped semicolons instead of silently dropping parameters.
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		a.writeProblem(w, http.StatusBadRequest, "malformed query string")
		return contacts.Query{}, false
	}
	query := contacts.Query{Limit: contacts.DefaultLimit, Sort: contacts.SortID}
	for field, entries := range values {
		if len(entries) != 1 {
			a.writeProblem(w, http.StatusBadRequest, "each query parameter must appear only once")
			return contacts.Query{}, false
		}
		value := entries[0]
		switch field {
		case "limit", "offset":
			number, err := strconv.Atoi(value)
			if err != nil || number < 0 || strconv.Itoa(number) != value {
				a.writeProblem(w, http.StatusBadRequest, field+" must be a nonnegative decimal integer without leading zeros")
				return contacts.Query{}, false
			}
			if field == "limit" {
				query.Limit = number
			} else {
				query.Offset = number
			}
		case "name_contains":
			query.NameContains = value
		case "sort":
			if value == "" {
				a.writeProblem(w, http.StatusBadRequest, "sort must not be empty")
				return contacts.Query{}, false
			}
			query.Sort = contacts.SortOrder(value)
		default:
			a.writeProblem(w, http.StatusBadRequest, "query may contain only limit, offset, name_contains, and sort")
			return contacts.Query{}, false
		}
	}
	return query, true // The store validates ranges, text, and the allowed orders.
}

func collectionURL(query contacts.Query, offset int) string {
	values := url.Values{
		"limit":  {strconv.Itoa(query.Limit)},
		"offset": {strconv.Itoa(offset)},
		"sort":   {string(query.Sort)},
	}
	if query.NameContains != "" {
		values.Set("name_contains", query.NameContains)
	}
	// Build relative URLs from accepted values; never copy the request's Host.
	return "/contacts?" + values.Encode()
}

func pageLinks(page contacts.Page) Links {
	query := page.Query
	links := Links{Self: collectionURL(query, query.Offset), First: collectionURL(query, 0)}
	if query.Offset > 0 {
		links.Prev = collectionURL(query, max(0, query.Offset-query.Limit))
	}
	if query.Offset < page.Total && query.Limit < page.Total-query.Offset {
		links.Next = collectionURL(query, query.Offset+query.Limit)
	}
	return links
}
