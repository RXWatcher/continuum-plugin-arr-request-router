package server

import "testing"

func TestComputeBaseHref(t *testing.T) {
	cases := []struct {
		name    string
		reqPath string
		want    string
	}{
		{"plugin root no trailing slash", "/admin", "./"},
		{"plugin root trailing slash", "/admin/", "../"},
		{"one level deep", "/admin/registry", "../"},
		{"two levels deep trailing slash", "/admin/registry/", "../../"},
		{"three levels deep file", "/admin/registry/123/edit", "../../../"},
		{"root only", "/", "./"},
		{"empty", "", "./"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeBaseHref(tc.reqPath)
			if got != tc.want {
				t.Errorf("computeBaseHref(%q) = %q, want %q", tc.reqPath, got, tc.want)
			}
		})
	}
}
