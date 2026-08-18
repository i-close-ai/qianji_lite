package cli

import "testing"

func TestCatalogCheckedToday(t *testing.T) {
	if !catalogCheckedToday("2026-08-18", "abc", "2026-08-18") {
		t.Fatal("same day with sha should skip fetch")
	}
	if catalogCheckedToday("2026-08-17", "abc", "2026-08-18") {
		t.Fatal("new day should fetch")
	}
	if catalogCheckedToday("2026-08-18", "", "2026-08-18") {
		t.Fatal("missing sha should fetch")
	}
}
