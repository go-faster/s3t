package harness

import (
	"regexp"
	"strings"
	"testing"
)

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	body := func(*T) {}
	r, err := NewRegistry([]Test{
		{Name: "bucket_list_empty", Module: ModuleS3, Fn: body},
		{Name: "bucket_list_many", Module: ModuleS3, Fn: body, Markers: []string{"list_objects_v2"}},
		{Name: "object_write", Module: ModuleS3, Fn: body, Markers: []string{"fails_on_aws"}},
		{Name: "object_create_bad_md5", Module: ModuleHeaders, Fn: body, Markers: []string{"auth_common"}},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

func names(tests []Test) string {
	out := make([]string, len(tests))
	for i, t := range tests {
		out[i] = t.Name
	}
	return strings.Join(out, ",")
}

func TestSelect(t *testing.T) {
	r := testRegistry(t)

	for _, tc := range []struct {
		name string
		sel  Selection
		want string
	}{
		{"everything", Selection{}, "bucket_list_empty,bucket_list_many,object_create_bad_md5,object_write"},
		{"by name", Selection{Run: regexp.MustCompile("^bucket_list")}, "bucket_list_empty,bucket_list_many"},
		{"by marker", Selection{Markers: mustExpr(t, "fails_on_aws")}, "object_write"},
		{"negated marker", Selection{Markers: mustExpr(t, "not fails_on_aws")},
			"bucket_list_empty,bucket_list_many,object_create_bad_md5"},
		{
			"allow-list",
			Selection{AllowList: []string{
				"s3tests/functional/test_s3.py::test_bucket_list_empty",
				"s3tests/functional/test_headers.py::test_object_create_bad_md5",
			}},
			"bucket_list_empty,object_create_bad_md5",
		},
		{
			// Criteria intersect rather than override.
			"allow-list and marker",
			Selection{
				AllowList: []string{
					"s3tests/functional/test_s3.py::test_bucket_list_empty",
					"s3tests/functional/test_s3.py::test_object_write",
				},
				Markers: mustExpr(t, "not fails_on_aws"),
			},
			"bucket_list_empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.Select(tc.sel)
			if err != nil {
				t.Fatalf("Select: %v", err)
			}
			if names(got) != tc.want {
				t.Errorf("Select = %q, want %q", names(got), tc.want)
			}
		})
	}
}

// A gate that silently stops covering a renamed test is worse than one that
// fails, so every unresolvable entry must be an error.
func TestSelectAllowListErrors(t *testing.T) {
	r := testRegistry(t)
	for _, tc := range []struct{ name, entry string }{
		{"not a node id", "test_bucket_list_empty"},
		{"unknown test", "s3tests/functional/test_s3.py::test_does_not_exist"},
		{"wrong module", "s3tests/functional/test_headers.py::test_bucket_list_empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := r.Select(Selection{AllowList: []string{tc.entry}}); err == nil {
				t.Errorf("Select accepted %q", tc.entry)
			}
		})
	}
}

func TestParseAllowList(t *testing.T) {
	got, err := ParseAllowList(strings.NewReader(`
# Allow-listed ceph/s3-tests node IDs.

s3tests/functional/test_s3.py::test_bucket_list_empty
   s3tests/functional/test_s3.py::test_object_write   # trailing comment

`))
	if err != nil {
		t.Fatalf("ParseAllowList: %v", err)
	}
	want := []string{
		"s3tests/functional/test_s3.py::test_bucket_list_empty",
		"s3tests/functional/test_s3.py::test_object_write",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ParseAllowList = %v, want %v", got, want)
	}
}

func TestParseAllowListEmpty(t *testing.T) {
	if _, err := ParseAllowList(strings.NewReader("# nothing but a comment\n")); err == nil {
		t.Error("ParseAllowList accepted a file with no entries")
	}
}

func mustExpr(t *testing.T, s string) MarkerExpr {
	t.Helper()
	e, err := ParseMarkerExpr(s)
	if err != nil {
		t.Fatalf("ParseMarkerExpr(%q): %v", s, err)
	}
	return e
}
