package serde

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/sdkerrors"
)

// TextKVName is an example extension codec demonstrating how downstream
// projects can plug in their own wire format.
//
// It encodes flat maps[string]any and simple scalar values as sorted
// "key=value" lines (one per line). It intentionally only supports flat
// scalar maps; anything else is a stable input error.
const TextKVName = "textkv"

// TextKVCodec is an example pluggable serializer.
type TextKVCodec struct{}

// Name implements Codec.
func (TextKVCodec) Name() string { return TextKVName }

// Marshal implements Codec.
func (TextKVCodec) Marshal(v any) ([]byte, error) {
	m, err := flatMap(v)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(escape(scalarString(m[k])))
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// Unmarshal implements Codec.
func (TextKVCodec) Unmarshal(data []byte, v any) error {
	m, err := flatMap(v)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return sdkerrors.Newf(sdkerrors.KindRuntime,
				"textkv decode: line %d is missing '='", i+1)
		}
		key := line[:eq]
		raw := line[eq+1:]
		val, err := unescape(raw)
		if err != nil {
			return sdkerrors.New(sdkerrors.KindRuntime,
				"textkv decode: invalid escaping").Wrap(err)
		}
		old, ok := m[key]
		if !ok {
			continue // keys not present in target are ignored
		}
		m[key] = coerce(val, old)
	}
	return nil
}

func flatMap(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, sdkerrors.New(sdkerrors.KindInput,
			"textkv codec only supports flat map[string]any payloads")
	}
	for k, val := range m {
		switch val.(type) {
		case string, bool, int, int64, float64, nil:
		default:
			return nil, sdkerrors.Newf(sdkerrors.KindInput,
				"textkv codec only supports scalar values (field %q has %T)", k, val)
		}
	}
	return m, nil
}

func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	default:
		return fmt.Sprint(t)
	}
}

func coerce(s string, old any) any {
	switch old.(type) {
	case bool:
		b, _ := strconv.ParseBool(s)
		return b
	case int:
		n, _ := strconv.Atoi(s)
		return n
	case int64:
		n, _ := strconv.ParseInt(s, 10, 64)
		return n
	case float64:
		f, _ := strconv.ParseFloat(s, 64)
		return f
	default:
		return s
	}
}

func escape(s string) string {
	r := strings.NewReplacer("\\", `\\`, "\n", `\n`, "=", `\e`)
	return r.Replace(s)
}

func unescape(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			return "", io.ErrUnexpectedEOF
		}
		switch s[i] {
		case '\\':
			b.WriteByte('\\')
		case 'n':
			b.WriteByte('\n')
		case 'e':
			b.WriteByte('=')
		default:
			return "", fmt.Errorf("bad escape \\%c", s[i])
		}
	}
	return b.String(), nil
}
