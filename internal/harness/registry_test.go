package harness

import "testing"

func TestNodeID(t *testing.T) {
	// Must reproduce the pytest node IDs used in allow-list files verbatim.
	got := Test{Name: "bucket_list_empty", Module: ModuleS3}.NodeID()
	const want = "s3tests/functional/test_s3.py::test_bucket_list_empty"
	if got != want {
		t.Fatalf("NodeID() = %q, want %q", got, want)
	}
}

func TestNewRegistryRejects(t *testing.T) {
	body := func(*T) {}
	for _, tc := range []struct {
		name  string
		tests []Test
	}{
		{"no name", []Test{{Module: ModuleS3, Fn: body}}},
		{"no body", []Test{{Name: "x", Module: ModuleS3}}},
		{"unknown module", []Test{{Name: "x", Module: "nope.py", Fn: body}}},
		{"duplicate", []Test{
			{Name: "x", Module: ModuleS3, Fn: body},
			{Name: "x", Module: ModuleHeaders, Fn: body},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewRegistry(tc.tests); err == nil {
				t.Fatal("NewRegistry accepted invalid input")
			}
		})
	}
}

func TestRegistryMarkers(t *testing.T) {
	body := func(*T) {}
	r, err := NewRegistry([]Test{
		{Name: "a", Module: ModuleS3, Fn: body, Markers: []string{"lifecycle", "fails_on_aws"}},
		{Name: "b", Module: ModuleS3, Fn: body, Markers: []string{"lifecycle"}},
		{Name: "c", Module: ModuleS3, Fn: body},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if r.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", r.Len())
	}

	got := r.Markers()
	want := []MarkerCount{{"fails_on_aws", 1}, {"lifecycle", 2}}
	if len(got) != len(want) {
		t.Fatalf("Markers() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Markers()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRegistryAllSorted(t *testing.T) {
	body := func(*T) {}
	r, err := NewRegistry([]Test{
		{Name: "c", Module: ModuleS3, Fn: body},
		{Name: "a", Module: ModuleS3, Fn: body},
		{Name: "b", Module: ModuleS3, Fn: body},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	for i, want := range []string{"a", "b", "c"} {
		if got := r.All()[i].Name; got != want {
			t.Errorf("All()[%d] = %q, want %q", i, got, want)
		}
	}
}
