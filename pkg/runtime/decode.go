package runtime

import (
	"encoding/json"
	"errors"

	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/apierrors"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/codegen"
	"github.com/highcumontoa/versioned-sdk-toolchain-go/pkg/schema"
)

func decodeMetadata(data []byte, md *codegen.Metadata) error {
	return json.Unmarshal(data, md)
}

func asError(err error, target **apierrors.Error) bool {
	return errors.As(err, target)
}

func metaToType(m codegen.MetaType) schema.TypeRef {
	t := schema.TypeRef{Nullable: m.Nullable}
	switch m.Kind {
	case "primitive":
		t.Primitive = schema.Primitive(m.Primitive)
	case "ref":
		t.Name = m.Name
	case "array":
		inner := metaToType(*m.Element)
		t.Array = &inner
	case "map":
		inner := metaToType(*m.Value)
		t.MapValue = &inner
	}
	return t
}

func bundleFromMetadata(md *codegen.Metadata) *schema.Bundle {
	ver, _ := schema.ParseVersion(md.Version)
	doc := &schema.Document{Service: md.Service, Version: ver, Source: "metadata.json"}
	for _, e := range md.Enums {
		doc.Enums = append(doc.Enums, schema.Enum{Name: e.Name, Values: append([]string(nil), e.Values...)})
	}
	for _, s := range md.Structs {
		st := schema.Struct{Name: s.Name}
		for _, f := range s.Fields {
			st.Fields = append(st.Fields, schema.Field{
				Name: f.Name, Type: metaToType(f.Type), Optional: f.Optional,
			})
		}
		doc.Structs = append(doc.Structs, st)
	}
	for _, i := range md.Interfaces {
		ifc := schema.Interface{Name: i.Name, Method: i.Method}
		for _, p := range i.Input {
			ifc.Input = append(ifc.Input, schema.Parameter{
				Name: p.Name, Type: metaToType(p.Type), Optional: p.Optional,
			})
		}
		if i.Output != nil {
			t := metaToType(*i.Output)
			ifc.Output = &t
		}
		for _, e := range i.Errors {
			ifc.Errors = append(ifc.Errors, metaToType(e))
		}
		doc.Interfaces = append(doc.Interfaces, ifc)
	}
	return &schema.Bundle{Documents: []*schema.Document{doc}}
}

// newOutputTarget returns a pointer to a generic decode container. The
// metadata-driven runtime decodes into generic containers (map / []any);
// callers wanting typed output use the generated client methods directly.
func newOutputTarget(t schema.TypeRef) any {
	switch t.Kind() {
	case "array":
		var v []any
		return &v
	case "map", "ref":
		var v map[string]any
		return &v
	default:
		var v any
		return &v
	}
}
