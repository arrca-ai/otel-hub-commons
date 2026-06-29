// SPDX-License-Identifier: Apache-2.0

// Package spanname derives canonical, instrumentation-independent span names and
// templatized attributes from OTLP/span attribute maps. It is graph-free (no
// entity/edge concepts) so both the flow side (flow node names) and the graph
// side (endpoint entity display names) can share one source of truth — if the
// two templatized span names differently, flow nodes and endpoint entities would
// disagree. The graph entity/edge derivation in internal/builder is layered on
// top of these helpers.
package spanname

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/arrca-ai/otel-hub-commons/bus"
)

const (
	attrHTTPMethod = "http.request.method"
	attrHTTPRoute  = "http.route"
	attrURLFull    = "url.full"
	attrRPCSystem  = "rpc.system"
	attrRPCService = "rpc.service"
	attrRPCMethod  = "rpc.method"
	attrSpanName   = "span.name"

	attrMsgSystem  = "messaging.system"
	attrMsgOp      = "messaging.operation"
	attrMsgOpType  = "messaging.operation.type" // newer semconv
	attrMsgDest    = "messaging.destination.name"
	attrMsgDestOld = "messaging.destination" // legacy
)

// SeriesAttrs rebuilds the templatized per-span attribute map the derivers
// expect: the span's own attributes plus the injected span.kind / span.name
// (top-level span fields, not attributes), run through TemplatizeAttrs — the same
// per-span buffer the graph previously built directly from OTLP spans. Both the
// flow and graph mappers share it so their span names agree.
func SeriesAttrs(s *bus.Span) map[string]string {
	a := s.GetAttrs()
	m := make(map[string]string, len(a)+2)
	for k, v := range a {
		m[k] = v
	}
	m["span.kind"] = s.GetKind() // hub already normalized to SERVER|CLIENT|...
	m["span.name"] = s.GetName()
	TemplatizeAttrs(m)
	return m
}

// TemplatizeAttrs collapses high-cardinality numeric IDs in the datapoint
// attributes that feed entity/edge derivation, in place. Intended to run
// once at ingest (in the receiver's collectRecords) so normalization happens
// at the boundary rather than scattered across the derivers. Only the two
// known high-cardinality keys are touched. Idempotent.
func TemplatizeAttrs(attrs map[string]string) {
	if v, ok := attrs[attrURLFull]; ok {
		attrs[attrURLFull] = templatizeURL(v)
	}
	if v, ok := attrs[attrSpanName]; ok {
		attrs[attrSpanName] = TemplatizeSpanName(v)
	}
}

// templatizeURL collapses numeric path segments of a URL (or bare path),
// dropping any query/fragment. Full URLs keep their scheme+host; reassembled
// manually because url.URL.String() would percent-encode the "{}" of "{n}".
func templatizeURL(s string) string {
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "/") {
		if i := strings.IndexAny(s, "?#"); i >= 0 {
			s = s[:i]
		}
		return templatizePath(s)
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return s
	}
	return u.Scheme + "://" + u.Host + templatizePath(u.Path)
}

// TemplatizeSpanName collapses numeric path segments in a span name so that
// names differing only by an integer id (e.g. "GET /orders/1" vs
// "GET /orders/2") canonicalize to the same value. Without it, every
// distinct id mints a new database QUERIES edge (the dedup key includes the
// action), defeating in-memory dedup since each is a genuinely distinct key.
//
// Rules (intentionally narrow):
//   - "/path/123"        -> "/path/{n}"        (bare path shape)
//   - "GET /path/123"    -> "GET /path/{n}"    (METHOD path shape)
//   - anything else (no leading "/", no "METHOD /path") is left untouched.
//
// Idempotent: already-templatized input is unchanged.
func TemplatizeSpanName(s string) string {
	if s == "" {
		return s
	}
	// Plain path shape: "/api/orders/123"
	if strings.HasPrefix(s, "/") {
		return templatizePath(s)
	}
	// "METHOD /path" shape: "GET /api/orders/123"
	sp := strings.IndexByte(s, ' ')
	if sp == -1 {
		return s
	}
	method, rest := s[:sp], s[sp+1:]
	if !strings.HasPrefix(rest, "/") {
		return s
	}
	return method + " " + templatizePath(rest)
}

// templatizePath rewrites every all-digit path segment to {n}. Idempotent.
func templatizePath(p string) string {
	if p == "" || p == "/" {
		return p
	}
	parts := strings.Split(p, "/")
	changed := false
	for i, seg := range parts {
		if isAllDigits(seg) {
			parts[i] = "{n}"
			changed = true
		}
	}
	if !changed {
		return p
	}
	return strings.Join(parts, "/")
}

// RPCSpanName returns a canonical "<rpc.system> /<rpc.service>/<rpc.method>" name
// for an RPC span (e.g. "grpc /oteldemo.CartService/GetCart"), or ok=false when
// the span is not RPC (no rpc.* and no gRPC-shaped route). It is the RPC analogue
// of HTTPSpanName and produces the same display the entity path uses for the RPC
// endpoint, so a flow node and its endpoint entity agree.
func RPCSpanName(attrs map[string]string) (string, bool) {
	system, route, ok := RPCSystemAndRoute(attrs)
	if !ok {
		return "", false
	}
	return system + " " + route, true
}

// HTTPSpanName returns a canonical "<METHOD> <route>" name for an HTTP span,
// reconstructed from http.request.method plus http.route (or, when absent, the
// path of url.full), with route placeholders and digit segments collapsed to
// {n}. ok is false when the record is not an HTTP span (no method, or no
// route/url).
//
// It gives flows a method-aware, instrumentation-independent node name: a
// gateway span named only by path (e.g. "/api/orders") becomes "GET /api/orders"
// — distinct from "POST /api/orders" — and a ":id"/digit route collapses the
// same way the entity path's endpoint display name does, so the two agree.
func HTTPSpanName(attrs map[string]string) (string, bool) {
	method := attrs[attrHTTPMethod]
	if method == "" {
		return "", false
	}
	route := attrs[attrHTTPRoute]
	if route == "" {
		route = ExtractURLPath(attrs[attrURLFull])
	}
	if route == "" {
		return "", false
	}
	return method + " " + NormalizeRoute(route), true
}

// MessagingSpanName returns a canonical "<system>.<operation> <destination>" name
// for a messaging span (e.g. "nats.publish metrics.part.{n}"), with numeric tokens
// in the destination — partition/shard indices, which are high-cardinality —
// collapsed to {n}. Without this, a fan-out that publishes to N partitioned
// subjects (metrics.part.0 … metrics.part.255) mints N distinct node names and
// never canonicalizes to a single fanned-out node. ok is false when the span is
// not a messaging span (no messaging.system, or no destination).
//
// The operation is optional: with it the name is "<system>.<operation> <dest>",
// without it just "<system> <dest>". destination prefers
// messaging.destination.name and falls back to the legacy messaging.destination;
// operation prefers messaging.operation and falls back to
// messaging.operation.type. Idempotent.
func MessagingSpanName(attrs map[string]string) (string, bool) {
	system := attrs[attrMsgSystem]
	if system == "" {
		return "", false
	}
	dest := attrs[attrMsgDest]
	if dest == "" {
		dest = attrs[attrMsgDestOld]
	}
	if dest == "" {
		return "", false
	}
	prefix := system
	op := attrs[attrMsgOp]
	if op == "" {
		op = attrs[attrMsgOpType]
	}
	if op != "" {
		prefix = system + "." + op
	}
	return prefix + " " + templatizeDestination(dest), true
}

// templatizeDestination collapses every all-digit token of a messaging
// destination to {n}. Tokens are delimited by the separators common to subjects
// and topics ('.', '/', '-', ':'); the separators are preserved. Idempotent
// ("{n}" is not all-digits, so re-applying is a no-op).
func templatizeDestination(dest string) string {
	var b strings.Builder
	b.Grow(len(dest) + 4)
	start := 0
	flush := func(end int) {
		if tok := dest[start:end]; isAllDigits(tok) {
			b.WriteString("{n}")
		} else {
			b.WriteString(tok)
		}
	}
	for i := 0; i < len(dest); i++ {
		switch dest[i] {
		case '.', '/', '-', ':':
			flush(i)
			b.WriteByte(dest[i])
			start = i + 1
		}
	}
	flush(len(dest))
	return b.String()
}

// RPCSystemAndRoute derives the RPC protocol and route-equivalent for a span,
// returning ok=false when the span is not RPC. It reads explicit rpc.* attributes
// (rpc.system default "rpc"; route /<rpc.service>/<rpc.method>) and — because some
// gRPC instrumentations report only HTTP semantics for what is really a gRPC call
// — falls back to a gRPC-shaped http.route or url.full path. That lets a server
// reporting rpc.* (or http-only) and its peer reporting the other convention
// still converge on one host-independent endpoint.
func RPCSystemAndRoute(attrs map[string]string) (system, route string, ok bool) {
	if method := attrs[attrRPCMethod]; method != "" {
		system = attrs[attrRPCSystem]
		if system == "" {
			system = "rpc"
		}
		route = "/" + method
		if svc := attrs[attrRPCService]; svc != "" {
			route = "/" + svc + "/" + method
		}
		return system, route, true
	}
	if r := grpcRouteFromHTTP(attrs); r != "" {
		return "grpc", r, true
	}
	return "", "", false
}

// grpcPathRe matches a gRPC request path /<pkg.Service>/<Method>, where the
// service segment is dot-qualified and ends in an UpperCamel name — strict enough
// to skip dotted REST routes like /v1.0/orders or /api.v2/health.
var grpcPathRe = regexp.MustCompile(`^/([a-z][\w.]*\.[A-Z]\w*)/([A-Za-z]\w*)$`)

// grpcRouteFromHTTP returns the normalized /<service>/<method> route when the
// span's http.route (server side) or url.full path (client side) is gRPC-shaped,
// or "" otherwise.
func grpcRouteFromHTTP(attrs map[string]string) string {
	for _, raw := range []string{attrs[attrHTTPRoute], ExtractURLPath(attrs[attrURLFull])} {
		if m := grpcPathRe.FindStringSubmatch(raw); m != nil {
			return "/" + m[1] + "/" + m[2]
		}
	}
	return ""
}

// ExtractURLPath returns the path component of a URL. If s already looks
// like a bare path ("/..."), it is returned as-is (sans query/fragment).
func ExtractURLPath(s string) string {
	if strings.HasPrefix(s, "/") {
		if i := strings.IndexAny(s, "?#"); i >= 0 {
			return s[:i]
		}
		return s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	return u.Path
}

// placeholderSegmentRe matches a single path segment that's a route
// placeholder in any common style: :foo, {foo}, <foo>. These come from
// framework-emitted http.route attributes (e.g. /api/orders/:id) and we
// normalize them so they collapse with the digit-templatized url.full
// path on the client side.
var placeholderSegmentRe = regexp.MustCompile(`^(?::[^/]+|\{[^/]+\}|<[^/]+>)$`)

// NormalizeRoute rewrites placeholder-style segments AND all-digit
// segments to {n}. Idempotent: re-applying to already-templatized input
// is a no-op.
func NormalizeRoute(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	parts := strings.Split(p, "/")
	changed := false
	for i, seg := range parts {
		if placeholderSegmentRe.MatchString(seg) || isAllDigits(seg) {
			parts[i] = "{n}"
			changed = true
		}
	}
	if !changed {
		return p
	}
	return strings.Join(parts, "/")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
