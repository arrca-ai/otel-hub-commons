// SPDX-License-Identifier: Apache-2.0

package spanname

import "testing"

func TestHTTPSpanName(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		want  string
		ok    bool
	}{
		{"gateway list (path-only name gains method)",
			map[string]string{"http.request.method": "GET", "http.route": "/api/orders"}, "GET /api/orders", true},
		{"gateway create separates from GET",
			map[string]string{"http.request.method": "POST", "http.route": "/api/orders"}, "POST /api/orders", true},
		{"route placeholder collapses to {n}",
			map[string]string{"http.request.method": "GET", "http.route": "/api/orders/:id"}, "GET /api/orders/{n}", true},
		{"falls back to url.full path, digits collapse",
			map[string]string{"http.request.method": "GET", "url.full": "http://order-service/api/orders/123"}, "GET /api/orders/{n}", true},
		{"non-http span -> not ok",
			map[string]string{"span.name": "SELECT auth.users", "db.system": "mysql"}, "", false},
		{"method but no route/url -> not ok",
			map[string]string{"http.request.method": "GET"}, "", false},
	}
	for _, c := range cases {
		got, ok := HTTPSpanName(c.attrs)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: HTTPSpanName = %q,%v; want %q,%v", c.name, got, ok, c.want, c.ok)
		}
	}
}
