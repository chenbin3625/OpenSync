package service

import "testing"

func TestBuildCronSpecFromSeparateFields(t *testing.T) {
	spec := buildCronSpec(map[string]interface{}{
		"second":      "0",
		"minute":      "*/5",
		"hour":        "*",
		"day":         "*",
		"month":       "*",
		"day_of_week": "*",
	})

	if spec != "0 */5 * * * *" {
		t.Fatalf("buildCronSpec() = %q, want %q", spec, "0 */5 * * * *")
	}
}

func TestBuildCronSpecRejectsEmptySchedule(t *testing.T) {
	if spec := buildCronSpec(map[string]interface{}{}); spec != "" {
		t.Fatalf("buildCronSpec(empty) = %q, want empty string", spec)
	}
}
