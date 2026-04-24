package eventbus

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/Wei-Shaw/sub2api/pkg/plugin"
)

// decodeIntoPayloadType takes a schema's PayloadExample and a JSON-encoded
// payload; it returns a freshly allocated value of the same underlying type
// populated from the JSON.
//
// Behaviour:
//   - If PayloadExample is a pointer (e.g. *Account), the returned value is
//     also a pointer of that type.
//   - If PayloadExample is a struct (e.g. OrderEvent), the returned value is
//     a struct value of that type.
//
// This is used by the AsyncDispatcher to feed typed payloads to handlers
// that were originally called with typed publishes.
func decodeIntoPayloadType(example any, raw json.RawMessage) (any, error) {
	t := reflect.TypeOf(example)
	if t == nil {
		return nil, fmt.Errorf("%w: nil payload example", plugin.ErrEventSchemaInvalid)
	}
	switch t.Kind() {
	case reflect.Ptr:
		return decodePointerPayload(t, raw)
	case reflect.Struct:
		return decodeStructPayload(t, raw)
	default:
		return nil, fmt.Errorf("%w: unsupported payload kind %s",
			plugin.ErrEventSchemaInvalid, t.Kind())
	}
}

func decodePointerPayload(t reflect.Type, raw json.RawMessage) (any, error) {
	ptr := reflect.New(t.Elem())
	if err := json.Unmarshal(raw, ptr.Interface()); err != nil {
		return nil, fmt.Errorf("%w: %v", plugin.ErrEventHandlerSignature, err)
	}
	return ptr.Interface(), nil
}

func decodeStructPayload(t reflect.Type, raw json.RawMessage) (any, error) {
	ptr := reflect.New(t)
	if err := json.Unmarshal(raw, ptr.Interface()); err != nil {
		return nil, fmt.Errorf("%w: %v", plugin.ErrEventHandlerSignature, err)
	}
	return ptr.Elem().Interface(), nil
}

// payloadMatchesSchema reports whether v is compatible with the schema's
// PayloadExample type. Used at Publish time to catch wrong-type payloads.
func payloadMatchesSchema(v any, example any) bool {
	if v == nil || example == nil {
		return v == nil && example == nil
	}
	exampleType := reflect.TypeOf(example)
	valueType := reflect.TypeOf(v)
	if exampleType == valueType {
		return true
	}
	// Allow addressable mismatches: example is struct, publish passes *struct.
	if exampleType.Kind() == reflect.Struct && valueType.Kind() == reflect.Ptr &&
		valueType.Elem() == exampleType {
		return true
	}
	if exampleType.Kind() == reflect.Ptr && valueType.Kind() == reflect.Struct &&
		exampleType.Elem() == valueType {
		return true
	}
	return false
}
