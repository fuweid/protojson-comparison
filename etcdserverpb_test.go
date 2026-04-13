package main

import (
	"testing"

	npb "github.com/fuweid/protojson-comparison/api/v3/etcdserverpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var localMarshalOptions = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: false,
}

func TestRPCResponseJSONLayout(t *testing.T) {
	responseNames := rpcResponseNames(npb.File_rpc_proto)
	if len(responseNames) == 0 {
		t.Fatal("no RPC response messages discovered")
	}

	helperPath := buildEtcdJSONHelper(t)

	for _, fullName := range responseNames {
		fullName := fullName
		t.Run(string(fullName), func(t *testing.T) {
			localMsg, err := newLocalMessage(fullName)
			if err != nil {
				t.Fatalf("create local message: %v", err)
			}

			// Fill one layer of fields, and one nested layer for message fields.
			populateMessage(localMsg.ProtoReflect(), 3)

			wire, err := proto.Marshal(localMsg)
			if err != nil {
				t.Fatalf("marshal local proto binary: %v", err)
			}

			// local package uses protojson over generated messages.
			localProtoJSON, err := localMarshalOptions.Marshal(localMsg)
			if err != nil {
				t.Fatalf("marshal local protojson: %v", err)
			}
			t.Logf("local protojson:\n%s", localProtoJSON)

			// etcd package uses encoding/json over etcd generated structs.
			etcdStyleJSON, err := marshalWithEtcdJSONHelper(helperPath, fullName, wire)
			if err != nil {
				t.Fatalf("marshal etcd-style json via helper: %v", err)
			}
			t.Logf("etcd-style json:\n%s", etcdStyleJSON)

			diff, err := compareJSONLayout(etcdStyleJSON, localProtoJSON)
			if err != nil {
				t.Fatalf("compare json with cmp: %v", err)
			}
			if diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
