package httpapi

import (
	"crypto/sha256"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"strings"

	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

// Bump this prefix if the contact JSON representation or its encoding changes.
// A domain version alone cannot distinguish two different wire representations.
const contactTagPrefix = "contact-json-v1-"

func contactTag(contact contacts.Contact) string {
	return `"` + contactTagPrefix + contacts.Version(contact) + `"`
}

type entityTag struct {
	opaque string
	weak   bool
}

type tagList struct {
	any  bool
	tags []entityTag
}

// Parse quoted entity tags, not comma-separated strings: a comma is valid
// inside an opaque tag. Backslashes are literal; they do not escape quotes.
func parseTagList(header http.Header, name string) (*tagList, error) {
	values := header.Values(name)
	if len(values) == 0 {
		return nil, nil
	}
	text := strings.Trim(strings.Join(values, ","), " \t")
	list := &tagList{}
	if text == "*" {
		list.any = true
		return list, nil
	}
	for len(text) > 0 {
		// Recipients tolerate empty list elements. An entirely empty list
		// matches nothing, which differs from an absent header.
		text = strings.TrimLeft(text, " \t,")
		if text == "" {
			break
		}
		tag := entityTag{}
		if strings.HasPrefix(text, "W/") {
			tag.weak = true
			text = text[2:]
		}
		if len(text) == 0 || text[0] != '"' {
			return nil, fmt.Errorf("%s must contain * or quoted entity tags", name)
		}
		text = text[1:]
		end := strings.IndexByte(text, '"')
		if end < 0 {
			return nil, fmt.Errorf("%s contains an unterminated entity tag", name)
		}
		for i := 0; i < end; i++ {
			b := text[i]
			if b < 0x21 || b == 0x7f {
				return nil, fmt.Errorf("%s contains an invalid entity tag", name)
			}
		}
		tag.opaque = text[:end]
		list.tags = append(list.tags, tag)
		text = strings.TrimLeft(text[end+1:], " \t")
		if text != "" && text[0] != ',' {
			return nil, fmt.Errorf("%s must separate entity tags with commas", name)
		}
	}
	return list, nil
}

func (list *tagList) matches(current string, strong bool) bool {
	if list.any {
		return true
	}
	for _, tag := range list.tags {
		if (!strong || !tag.weak) && `"`+tag.opaque+`"` == current {
			return true
		}
	}
	return false
}

func domainMatch(list *tagList, strong bool) *contacts.VersionMatch {
	if list == nil {
		return nil
	}
	match := &contacts.VersionMatch{Any: list.any}
	for _, tag := range list.tags {
		if (!strong || !tag.weak) && strings.HasPrefix(tag.opaque, contactTagPrefix) {
			match.Versions = append(match.Versions, strings.TrimPrefix(tag.opaque, contactTagPrefix))
		}
	}
	return match
}

func (a *api) readConditions(w http.ResponseWriter, r *http.Request) (contacts.Conditions, bool) {
	match, err := parseTagList(r.Header, "If-Match")
	if err != nil {
		a.writeProblem(w, http.StatusBadRequest, err.Error())
		return contacts.Conditions{}, false
	}
	none, err := parseTagList(r.Header, "If-None-Match")
	if err != nil {
		a.writeProblem(w, http.StatusBadRequest, err.Error())
		return contacts.Conditions{}, false
	}
	return contacts.Conditions{Match: domainMatch(match, true), NoneMatch: domainMatch(none, false)}, true
}

func (a *api) rejectConditions(w http.ResponseWriter, r *http.Request) bool {
	if len(r.Header.Values("If-Match"))+len(r.Header.Values("If-None-Match")) > 0 {
		a.writeProblem(w, http.StatusBadRequest, "entity-tag conditions are supported on GET, HEAD, and contact PUT, PATCH, DELETE")
		return true
	}
	return false
}

func (a *api) writeReadJSON(w http.ResponseWriter, r *http.Request, value any, tag string) {
	data, err := json.Marshal(value, json.Deterministic(true))
	if err != nil {
		a.logger.Error("encode response", "error", err)
		a.writeProblem(w, http.StatusInternalServerError, "response could not be encoded")
		return
	}
	a.writeReadBytes(w, r, append(data, '\n'), tag)
}

func (a *api) writeReadBytes(w http.ResponseWriter, r *http.Request, data []byte, tag string) {
	match, err := parseTagList(r.Header, "If-Match")
	if err != nil {
		a.writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	none, err := parseTagList(r.Header, "If-None-Match")
	if err != nil {
		a.writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if tag == "" {
		// Pages and the contract use a hash of their actual response bytes.
		tag = fmt.Sprintf(`"json-%x"`, sha256.Sum256(data))
	}
	if match != nil && !match.matches(tag, true) {
		a.writeProblem(w, http.StatusPreconditionFailed, "representation no longer matches If-Match")
		return
	}
	w.Header().Set("ETag", tag)
	// A private cache may store the representation but must revalidate it.
	w.Header().Set("Cache-Control", "private, no-cache")
	if none != nil && none.matches(tag, false) {
		w.WriteHeader(http.StatusNotModified) // No content type or body on 304.
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		a.logger.Error("write response", "error", err)
	}
}
