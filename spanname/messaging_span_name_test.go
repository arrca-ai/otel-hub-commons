// SPDX-License-Identifier: Apache-2.0

package spanname

import "testing"

func TestMessagingSpanName(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		want  string
		ok    bool
	}{
		{"nats publish with partition index",
			map[string]string{"messaging.system": "nats", "messaging.operation": "publish", "messaging.destination.name": "metrics.part.58"},
			"nats.publish metrics.part.{n}", true},
		{"multiple numeric tokens collapse",
			map[string]string{"messaging.system": "nats", "messaging.operation": "publish", "messaging.destination.name": "shard.12.queue.3"},
			"nats.publish shard.{n}.queue.{n}", true},
		{"no operation -> system + destination only",
			map[string]string{"messaging.system": "kafka", "messaging.destination.name": "orders"},
			"kafka orders", true},
		{"operation.type fallback (newer semconv)",
			map[string]string{"messaging.system": "kafka", "messaging.operation.type": "receive", "messaging.destination.name": "orders.7"},
			"kafka.receive orders.{n}", true},
		{"legacy messaging.destination key",
			map[string]string{"messaging.system": "nats", "messaging.operation": "publish", "messaging.destination": "metrics.part.9"},
			"nats.publish metrics.part.{n}", true},
		{"slash-delimited destination",
			map[string]string{"messaging.system": "nats", "messaging.operation": "send", "messaging.destination.name": "metrics/58/raw"},
			"nats.send metrics/{n}/raw", true},
		{"non-digit tokens preserved",
			map[string]string{"messaging.system": "nats", "messaging.operation": "publish", "messaging.destination.name": "metrics.part.abc"},
			"nats.publish metrics.part.abc", true},
		{"idempotent on already-templatized destination",
			map[string]string{"messaging.system": "nats", "messaging.operation": "publish", "messaging.destination.name": "metrics.part.{n}"},
			"nats.publish metrics.part.{n}", true},
		{"not a messaging span (no system)",
			map[string]string{"messaging.destination.name": "metrics.part.58"}, "", false},
		{"no destination -> not ok",
			map[string]string{"messaging.system": "nats", "messaging.operation": "publish"}, "", false},
	}
	for _, c := range cases {
		got, ok := MessagingSpanName(c.attrs)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: MessagingSpanName = %q,%v; want %q,%v", c.name, got, ok, c.want, c.ok)
		}
	}
}
