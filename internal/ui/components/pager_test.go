package components

import (
	"strings"
	"testing"
)


func TestPagesWindow(t *testing.T) {
	labels := func(items []pageItem) []string {
		out := make([]string, 0, len(items))
		for _, it := range items {
			if it.Ellipsis {
				out = append(out, "...")
			} else {
				out = append(out, it.Label)
			}
		}
		return out
	}
	cases := []struct {
		page, total int
		want        []string
	}{
		{1, 1, []string{"1"}},
		{1, 5, []string{"1", "2", "3", "4", "5"}},
		{1, 8, []string{"1", "2", "3", "...", "8"}},
		{4, 8, []string{"1", "...", "3", "4", "5", "...", "8"}},
		{5, 8, []string{"1", "...", "4", "5", "6", "...", "8"}},
		{8, 8, []string{"1", "...", "6", "7", "8"}},
		{99, 99, []string{"1", "...", "97", "98", "99"}},
	}
	for _, tc := range cases {
		got := labels(pagesWindow(tc.page, tc.total))
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("pagesWindow(%d, %d) = %v, want %v", tc.page, tc.total, got, tc.want)
		}
	}
}
