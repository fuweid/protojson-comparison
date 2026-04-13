module github.com/fuweid/protojson-comparison

go 1.26.0

toolchain go1.26.2

require (
	github.com/fuweid/protojson-comparison/api/v3 v3.0.0
	github.com/golang/protobuf v1.5.4
	github.com/google/go-cmp v0.7.0
	go.etcd.io/etcd/api/v3 v3.6.10
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260209200024-4cfbd4190f57 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
	google.golang.org/grpc v1.79.3 // indirect
)

replace github.com/fuweid/protojson-comparison/api/v3 => ./api
