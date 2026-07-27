package fixture

import "testing"

func TestSlug(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"bucket_list_empty", "bucket-list-empt"},
		{"bucket_list_delimiter_prefix_underscore", "bucket-list-deli"},
		{"a", "a"},
		{"UPPER_case", "upper-case"},
		{"__leading", "leading"},
		{"trailing__", "trailing"},
		{"100_continue", "100-continue"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got := slug(tc.in)
			if got != tc.want {
				t.Errorf("slug(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if len(got) > maxSlugLen {
				t.Errorf("slug(%q) = %q, longer than %d", tc.in, got, maxSlugLen)
			}
		})
	}
}

// A bucket name must stay within the 63-character S3 limit even with the
// longest config prefix and the longest upstream test name.
func TestBucketNameFits(t *testing.T) {
	const longestConfigPrefix = 30
	name := longestConfigPrefix + len(slug("bucket_list_delimiter_prefix_underscore")) +
		len("-") + len(token()) + len("-") + len("999")
	if name > 63 {
		t.Errorf("bucket name length %d exceeds the S3 limit of 63", name)
	}
}
