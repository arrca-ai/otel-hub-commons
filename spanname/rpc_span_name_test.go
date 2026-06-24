// SPDX-License-Identifier: Apache-2.0

package spanname

import "testing"

func TestRPCSpanName(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		want  string
		ok    bool
	}{
		{"grpc service + method",
			map[string]string{"rpc.system": "grpc", "rpc.service": "oteldemo.CartService", "rpc.method": "GetCart"},
			"grpc /oteldemo.CartService/GetCart", true},
		{"method only (no rpc.service)",
			map[string]string{"rpc.system": "grpc", "rpc.method": "GetCart"}, "grpc /GetCart", true},
		{"missing rpc.system defaults to rpc",
			map[string]string{"rpc.service": "foo.Bar", "rpc.method": "Baz"}, "rpc /foo.Bar/Baz", true},
		{"gRPC-shaped http.route, no rpc.* -> inferred grpc",
			map[string]string{"http.request.method": "POST", "http.route": "/oteldemo.CartService/GetCart"},
			"grpc /oteldemo.CartService/GetCart", true},
		{"plain http route -> not ok",
			map[string]string{"http.request.method": "GET", "http.route": "/x"}, "", false},
		{"dotted REST route -> not ok",
			map[string]string{"http.request.method": "GET", "http.route": "/v1.0/orders"}, "", false},
		{"no rpc.method -> not ok",
			map[string]string{"rpc.system": "grpc", "rpc.service": "foo.Bar"}, "", false},
	}
	for _, c := range cases {
		got, ok := RPCSpanName(c.attrs)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: RPCSpanName = %q,%v; want %q,%v", c.name, got, ok, c.want, c.ok)
		}
	}
}
