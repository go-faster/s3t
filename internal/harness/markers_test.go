package harness

import "testing"

func TestMarkerExpr(t *testing.T) {
	for _, tc := range []struct {
		expr    string
		markers []string
		want    bool
	}{
		// Empty selects everything.
		{"", nil, true},
		{"   ", []string{"lifecycle"}, true},

		{"lifecycle", []string{"lifecycle"}, true},
		{"lifecycle", []string{"tagging"}, false},
		{"lifecycle", nil, false},

		{"not lifecycle", []string{"lifecycle"}, false},
		{"not lifecycle", nil, true},
		{"not not lifecycle", []string{"lifecycle"}, true},

		{"a and b", []string{"a", "b"}, true},
		{"a and b", []string{"a"}, false},
		{"a or b", []string{"b"}, true},
		{"a or b", nil, false},

		// "and" binds tighter than "or".
		{"a or b and c", []string{"a"}, true},
		{"a or b and c", []string{"b"}, false},
		{"a or b and c", []string{"b", "c"}, true},

		// "not" binds tighter than "and".
		{"not a and b", []string{"b"}, true},
		{"not a and b", []string{"a", "b"}, false},

		{"(a or b) and c", []string{"a", "c"}, true},
		{"(a or b) and c", []string{"c"}, false},

		// The invocations upstream documents.
		{
			"bucket_logging and not fails_without_logging_rollover",
			[]string{"bucket_logging"}, true,
		},
		{
			"bucket_logging and not fails_without_logging_rollover",
			[]string{"bucket_logging", "fails_without_logging_rollover"}, false,
		},
		{"not fails_on_aws", []string{"lifecycle"}, true},
	} {
		t.Run(tc.expr+"/"+join(tc.markers), func(t *testing.T) {
			e, err := ParseMarkerExpr(tc.expr)
			if err != nil {
				t.Fatalf("ParseMarkerExpr(%q): %v", tc.expr, err)
			}
			if got := e.Match(tc.markers); got != tc.want {
				t.Errorf("Match(%v) = %v, want %v", tc.markers, got, tc.want)
			}
		})
	}
}

func TestMarkerExprInvalid(t *testing.T) {
	for _, expr := range []string{
		"(a",
		"a)",
		"a and",
		"and a",
		"not",
		"a b c and",
		"a & b",
		"()",
	} {
		t.Run(expr, func(t *testing.T) {
			if _, err := ParseMarkerExpr(expr); err == nil {
				t.Errorf("ParseMarkerExpr(%q) accepted invalid expression", expr)
			}
		})
	}
}

func join(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	out := s[0]
	for _, v := range s[1:] {
		out += "+" + v
	}
	return out
}
