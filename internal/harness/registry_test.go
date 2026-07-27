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

func TestRegisterRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		test Test
	}{
		{"no name", Test{Module: ModuleS3, Fn: func(*T) {}}},
		{"no body", Test{Name: "x", Module: ModuleS3}},
		{"unknown module", Test{Name: "x", Module: "nope.py", Fn: func(*T) {}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Register did not panic")
				}
			}()
			Register(tc.test)
		})
	}
}

func TestRegisterDuplicate(t *testing.T) {
	tc := Test{Name: "duplicate_probe", Module: ModuleS3, Fn: func(*T) {}}
	Register(tc)
	defer func() {
		if recover() == nil {
			t.Fatal("Register accepted a duplicate name")
		}
	}()
	Register(tc)
}
