package mapper

import "testing"

func TestParsePageParamsRejectsInvalidValues(t *testing.T) {
	cases := []map[string]interface{}{
		{"pageSize": "0", "pageNum": "1"},
		{"pageSize": "-1", "pageNum": "1"},
		{"pageSize": "20", "pageNum": "0"},
		{"pageSize": "abc", "pageNum": "1"},
		{"pageSize": "20", "pageNum": "abc"},
	}

	for _, params := range cases {
		if _, _, _, err := parsePageParams(params); err == nil {
			t.Fatalf("parsePageParams(%v) returned nil error, want error", params)
		}
	}
}

func TestParsePageParamsCapsLargePageSize(t *testing.T) {
	pageSize, pageNum, ok, err := parsePageParams(map[string]interface{}{
		"pageSize": "9999",
		"pageNum":  "2",
	})
	if err != nil {
		t.Fatalf("parsePageParams() error: %v", err)
	}
	if !ok {
		t.Fatalf("parsePageParams() ok = false, want true")
	}
	if pageSize != maxPageSize {
		t.Fatalf("pageSize = %d, want capped maxPageSize %d", pageSize, maxPageSize)
	}
	if pageNum != 2 {
		t.Fatalf("pageNum = %d, want 2", pageNum)
	}
}

func TestParsePageParamsAllowsUnpaginatedRequests(t *testing.T) {
	_, _, ok, err := parsePageParams(map[string]interface{}{})
	if err != nil {
		t.Fatalf("parsePageParams(empty) error: %v", err)
	}
	if ok {
		t.Fatalf("parsePageParams(empty) ok = true, want false")
	}
}
