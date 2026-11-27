package pricing

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCostAndPriceRoutesReadVariantID pins the defect: the routes declare {variantId} and the
// handlers read PathValue("id"), so every cost and price record call answered 404 on a real
// variant — the whole pricing subsystem was unreachable from the panel.
func TestCostAndPriceRoutesReadVariantID(t *testing.T) {
	cases := []struct{ pattern, url string }{
		{"POST /api/v1/admin/variants/{variantId}/cost-records", "/api/v1/admin/variants/v-1/cost-records"},
		{"GET /api/v1/admin/variants/{variantId}/cost-records", "/api/v1/admin/variants/v-2/cost-records"},
		{"POST /api/v1/admin/variants/{variantId}/price-records", "/api/v1/admin/variants/v-3/price-records"},
		{"GET /api/v1/admin/variants/{variantId}/price-records", "/api/v1/admin/variants/v-4/price-records"},
	}
	for _, c := range cases {
		mux := http.NewServeMux()
		var got string
		mux.HandleFunc(c.pattern, func(w http.ResponseWriter, r *http.Request) {
			got = pathID(r, "variantId", "id")
		})
		method := c.pattern[:strings.Index(c.pattern, " ")]
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, c.url, nil))
		want := c.url[strings.Index(c.url, "/variants/")+len("/variants/"):]
		want = want[:strings.Index(want, "/")]
		if got != want {
			t.Fatalf("%s -> %q, want %q", c.pattern, got, want)
		}
	}
}
