package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// compareJSONLayout compares the JSON layout of two JSON strings, ignoring
// field order and insignificant whitespace.
func compareJSONLayout(leftJSON, rightJSON []byte) (string, error) {
	leftLayout, err := parseLayout(leftJSON)
	if err != nil {
		return "", err
	}
	rightLayout, err := parseLayout(rightJSON)
	if err != nil {
		return "", err
	}
	return cmp.Diff(leftLayout, rightLayout), nil
}

// parseLayout parses raw JSON bytes and converts them into a normalized layout
// tree by delegating to normalizeLayout.
//
// Return shapes:
//   - map[string]any for JSON objects
//   - []any for JSON arrays
//   - string type labels for scalars (for example: "string", "number")
func parseLayout(raw []byte) (any, error) {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	return normalizeLayout(parsed), nil
}

// normalizeLayout recursively normalizes a parsed JSON value to a structure
// suitable for layout-oriented comparison:
//   - object values keep their key structure
//   - array values preserve all normalized elements (to capture mixed types)
//   - scalar values are replaced by type labels via scalarTypeName
func normalizeLayout(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, child := range x {
			out[k] = normalizeLayout(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, elem := range x {
			out = append(out, normalizeLayout(elem))
		}
		return out
	default:
		return scalarTypeName(x)
	}
}

// scalarTypeName maps a scalar JSON value to a stable type label.
// json.Unmarshal produces scalars as:
//   - nil, bool, string, float64
func scalarTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case string:
		return "string"
	case float64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// rpcResponseNames returns all unique RPC response message names declared in
// the services of the provided proto file descriptor.
func rpcResponseNames(fd protoreflect.FileDescriptor) []protoreflect.FullName {
	seen := make(map[protoreflect.FullName]struct{})
	services := fd.Services()
	for i := 0; i < services.Len(); i++ {
		methods := services.Get(i).Methods()
		for j := 0; j < methods.Len(); j++ {
			seen[methods.Get(j).Output().FullName()] = struct{}{}
		}
	}

	out := make([]protoreflect.FullName, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// newLocalMessage constructs a new local protobuf message by full name.
func newLocalMessage(name protoreflect.FullName) (proto.Message, error) {
	mt, err := protoregistry.GlobalTypes.FindMessageByName(name)
	if err != nil {
		return nil, fmt.Errorf("find local message type %q: %w", name, err)
	}
	return mt.New().Interface(), nil
}

// buildEtcdJSONHelper builds the helper binary that uses etcd generated
// structs and encoding/json/v2 to marshal JSON.
func buildEtcdJSONHelper(t *testing.T) string {
	t.Helper()

	output := filepath.Join(t.TempDir(), "etcdjsonmarshal")
	cmd := exec.Command("go", "build", "-o", output, "./tools/etcdjsonmarshal")
	cmd.Env = append(os.Environ(), "GOEXPERIMENT=jsonv2")
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build etcd json helper failed: %v\n%s", err, string(combined))
	}
	return output
}

// marshalWithEtcdJSONHelper marshals wire protobuf into JSON using the etcd
// helper binary. Set ETCDJSON_STRINGIFY_NUMBERS=1 to enable json/v2
// StringifyNumbers in the helper.
func marshalWithEtcdJSONHelper(helperPath string, fullName protoreflect.FullName, wire []byte) ([]byte, error) {
	wireB64 := base64.StdEncoding.EncodeToString(wire)
	cmd := exec.Command(helperPath, string(fullName), wireB64)
	cmd.Env = os.Environ()
	combined, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("exec helper failed: %w (%s)", err, string(combined))
	}
	return combined, nil
}

// populateMessage recursively populates protobuf fields up to the given depth.
// It fills one field in each real oneof and all non-oneof fields.
func populateMessage(msg protoreflect.Message, depth int) {
	md := msg.Descriptor()

	oneofs := md.Oneofs()
	for i := 0; i < oneofs.Len(); i++ {
		od := oneofs.Get(i)
		if od.IsSynthetic() || od.Fields().Len() == 0 {
			continue
		}
		setField(msg, od.Fields().Get(0), depth)
	}

	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if od := fd.ContainingOneof(); od != nil && !od.IsSynthetic() {
			continue
		}
		setField(msg, fd, depth)
	}
}

func setField(msg protoreflect.Message, fd protoreflect.FieldDescriptor, depth int) {
	switch {
	case fd.IsList():
		list := msg.Mutable(fd).List()
		val, ok := sampleListValue(list, fd, depth)
		if ok {
			list.Append(val)
		}
	case fd.IsMap():
		mp := msg.Mutable(fd).Map()
		key, ok := sampleMapKey(fd.MapKey())
		if !ok {
			return
		}
		val, ok := sampleMapValue(mp, fd.MapValue(), depth)
		if !ok {
			return
		}
		mp.Set(key, val)
	case fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind:
		if depth <= 0 {
			return
		}
		sub := msg.Mutable(fd).Message()
		populateMessage(sub, depth-1)
	case fd.Kind() == protoreflect.EnumKind:
		msg.Set(fd, protoreflect.ValueOfEnum(sampleEnum(fd.Enum())))
	default:
		val, ok := sampleScalarValue(fd.Kind())
		if ok {
			msg.Set(fd, val)
		}
	}
}

func sampleListValue(list protoreflect.List, fd protoreflect.FieldDescriptor, depth int) (protoreflect.Value, bool) {
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if depth <= 0 {
			return protoreflect.Value{}, false
		}
		val := list.NewElement()
		populateMessage(val.Message(), depth-1)
		return val, true
	case protoreflect.EnumKind:
		return protoreflect.ValueOfEnum(sampleEnum(fd.Enum())), true
	default:
		return sampleScalarValue(fd.Kind())
	}
}

func sampleMapKey(fd protoreflect.FieldDescriptor) (protoreflect.MapKey, bool) {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("k").MapKey(), true
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true).MapKey(), true
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(1).MapKey(), true
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(1).MapKey(), true
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(1).MapKey(), true
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(1).MapKey(), true
	default:
		return protoreflect.MapKey{}, false
	}
}

func sampleMapValue(mp protoreflect.Map, fd protoreflect.FieldDescriptor, depth int) (protoreflect.Value, bool) {
	switch fd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if depth <= 0 {
			return protoreflect.Value{}, false
		}
		val := mp.NewValue()
		populateMessage(val.Message(), depth-1)
		return val, true
	case protoreflect.EnumKind:
		return protoreflect.ValueOfEnum(sampleEnum(fd.Enum())), true
	default:
		return sampleScalarValue(fd.Kind())
	}
}

func sampleEnum(ed protoreflect.EnumDescriptor) protoreflect.EnumNumber {
	if ed.Values().Len() > 1 {
		return ed.Values().Get(1).Number()
	}
	if ed.Values().Len() == 1 {
		return ed.Values().Get(0).Number()
	}
	return 0
}

func sampleScalarValue(kind protoreflect.Kind) (protoreflect.Value, bool) {
	switch kind {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true), true
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("x"), true
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte("x")), true
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(1), true
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(1), true
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(1), true
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(1), true
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(1.25), true
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(1.25), true
	default:
		return protoreflect.Value{}, false
	}
}
