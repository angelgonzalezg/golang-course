package rpcapi

import (
	contactsv1 "example.com/course/grpc-lab/gen/contacts/v1"
	"golangcourse/chapters/13-standard-library/06-project-rest-api/contacts"
)

func versionCondition(version *string) (contacts.Conditions, error) {
	if version == nil {
		return contacts.Conditions{}, nil // Absent means explicitly unconditional.
	}
	if *version == "" {
		return contacts.Conditions{}, invalid("expected_version", "must not be empty when present")
	}
	// Versions are opaque. Do not trim, remove quotes, or accept HTTP wildcards.
	return contacts.Conditions{Match: &contacts.VersionMatch{Versions: []string{*version}}}, nil
}

func selectedChanges(request *contactsv1.UpdateContactRequest) (contacts.Changes, error) {
	if request.UpdateMask == nil || len(request.UpdateMask.Paths) == 0 {
		return contacts.Changes{}, invalid("update_mask", "must select at least one editable field")
	}
	changes := contacts.Changes{}
	seen := make(map[string]bool, 3)
	for _, path := range request.UpdateMask.Paths {
		if seen[path] {
			return contacts.Changes{}, invalid("update_mask", "must not contain duplicate paths")
		}
		seen[path] = true
		switch path {
		case "name":
			value := request.Input.GetName()
			changes.Name = &value
		case "email":
			value := request.Input.GetEmail()
			changes.Email = &value
		case "phone":
			value := request.Input.GetPhone()
			changes.Phone = &value
		default:
			return contacts.Changes{}, invalid("update_mask", "may select only name, email, and phone")
		}
	}
	// Unselected values are ignored. Selected empty values remain non-nil
	// changes, so the domain can distinguish clearing data from keeping it.
	return changes, nil
}

func listQuery(request *contactsv1.ListContactsRequest) (contacts.Query, error) {
	limit := int(request.GetLimit())
	if limit == 0 {
		limit = contacts.DefaultLimit
	}
	offset := request.GetOffset()
	if offset < 0 || int64(int(offset)) != offset {
		return contacts.Query{}, invalid("offset", "must be nonnegative and fit the server's Go int")
	}
	query := contacts.Query{Limit: limit, Offset: int(offset), NameContains: request.GetNameContains()}
	switch request.GetSort() {
	case contactsv1.SortOrder_SORT_ORDER_UNSPECIFIED, contactsv1.SortOrder_SORT_ORDER_ID_ASC:
		query.Sort = contacts.SortID
	case contactsv1.SortOrder_SORT_ORDER_ID_DESC:
		query.Sort = contacts.SortIDDesc
	case contactsv1.SortOrder_SORT_ORDER_NAME_ASC:
		query.Sort = contacts.SortName
	case contactsv1.SortOrder_SORT_ORDER_NAME_DESC:
		query.Sort = contacts.SortNameDesc
	default:
		return contacts.Query{}, invalid("sort", "must be a defined SortOrder")
	}
	return query, nil // The shared store validates limits and filter text.
}
