.PHONY: proto
proto:
	protoc --go_out=. --go_opt=module=github.com/arrca-ai/otel-hub-commons \
	  bus/bus.proto
