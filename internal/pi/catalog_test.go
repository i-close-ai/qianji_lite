package pi_test

import (
	"strings"
	"testing"

	"github.com/i-close-ai/qianji_lite/internal/pi"
)

func TestParseListModels(t *testing.T) {
	text := strings.Join([]string{
		"provider   model                       context  max-out  thinking  images",
		"acme       fast                        1.0M     128K     yes       no",
		"acme       pro                         1.0M     128K     yes       no",
		"Warning: something",
		"",
		"acme       pro                         1.0M     128K     yes       no",
	}, "\n")
	catalog := pi.ParseListModels(text)
	if len(catalog) != 2 {
		t.Fatalf("len=%d %+v", len(catalog), catalog)
	}
	if catalog[0].Provider != "acme" || catalog[1].Model != "pro" {
		t.Fatalf("%+v", catalog)
	}
}

func TestCatalogSHA256Stable(t *testing.T) {
	a := pi.ParseListModels("acme pro\nacme fast\n")
	b := pi.ParseListModels("acme fast\nacme pro\n")
	if pi.CatalogSHA256(a) != pi.CatalogSHA256(b) {
		t.Fatal("order should not change digest")
	}
}

func TestLooksLikeFailure(t *testing.T) {
	if !pi.LooksLikeFailure(1, "ok") {
		t.Fatal("nonzero exit")
	}
	if !pi.LooksLikeFailure(0, "") {
		t.Fatal("empty")
	}
	if !pi.LooksLikeFailure(0, "Error: HTTP 403 Forbidden") {
		t.Fatal("http prefix")
	}
	if pi.LooksLikeFailure(0, "patched the tests successfully") {
		t.Fatal("false positive")
	}
	if pi.LooksLikeFailure(0, "The handler returns unauthorized for missing tokens.\nThen it writes the tests.") {
		t.Fatal("unauthorized in task output should not count as provider failure")
	}
	if !pi.LooksLikeFailure(0, "No API key found for provider acme") {
		t.Fatal("api key head")
	}
}
