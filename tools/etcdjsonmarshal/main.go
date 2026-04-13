package main

import (
	"encoding/base64"
	jsonv2 "encoding/json/v2"
	"fmt"
	"os"
	"reflect"

	goproto "github.com/golang/protobuf/proto"
	_ "go.etcd.io/etcd/api/v3/etcdserverpb"
)

const envStringifyNumbers = "ETCDJSON_STRINGIFY_NUMBERS"

func main() {
	if len(os.Args) != 3 {
		fatalf("usage: %s <full_message_name> <wire_base64>", os.Args[0])
	}

	fullName := os.Args[1]
	msgType := goproto.MessageType(fullName)
	if msgType == nil {
		fatalf("message type not found: %s", fullName)
	}

	if msgType.Kind() != reflect.Ptr || msgType.Elem().Kind() != reflect.Struct {
		fatalf("unexpected message type %s: %v", fullName, msgType)
	}

	msg, ok := reflect.New(msgType.Elem()).Interface().(goproto.Message)
	if !ok {
		fatalf("message type does not implement proto.Message: %s", fullName)
	}

	wire, err := base64.StdEncoding.DecodeString(os.Args[2])
	if err != nil {
		fatalf("decode wire: %v", err)
	}

	if err := goproto.Unmarshal(wire, msg); err != nil {
		fatalf("unmarshal wire: %v", err)
	}

	var out []byte
	if enableStringifyNumbers(os.Getenv(envStringifyNumbers)) {
		out, err = jsonv2.Marshal(msg, jsonv2.StringifyNumbers(true))
	} else {
		out, err = jsonv2.Marshal(msg)
	}
	if err != nil {
		fatalf("json marshal: %v", err)
	}

	if _, err := os.Stdout.Write(out); err != nil {
		fatalf("write stdout: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func enableStringifyNumbers(v string) bool {
	switch v {
	case "1", "true", "TRUE", "True", "yes", "YES", "Yes", "on", "ON", "On":
		return true
	default:
		return false
	}
}
