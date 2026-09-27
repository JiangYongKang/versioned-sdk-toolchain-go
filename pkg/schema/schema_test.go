package schema

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
		major   int
	}{
		{"1.2.3", false, 1},
		{"0.0.0", false, 0},
		{"1.0", true, 0},
		{"v1.0.0", true, 0},
		{"1.0.x", true, 0},
		{"01.0.0", true, 0},
		{"1.0.-1", true, 0},
		{"", true, 0},
	}
	for _, c := range cases {
		v, err := ParseVersion(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("ParseVersion(%q) expected error, got %+v", c.in, v)
			}
			t.Logf("rejected illegal version %q: %v", c.in, err)
			continue
		}
		if err != nil {
			t.Fatalf("ParseVersion(%q) unexpected error: %v", c.in, err)
		}
		if v.Major != c.major || v.String() != c.in {
			t.Fatalf("ParseVersion(%q) = %+v", c.in, v)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	v := func(s string) Version {
		x, err := ParseVersion(s)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	pairs := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.99.99", 1},
	}
	for _, p := range pairs {
		if got := v(p.a).Compare(v(p.b)); got != p.want {
			t.Fatalf("%s.Compare(%s) = %d, want %d", p.a, p.b, got, p.want)
		}
	}
	t.Logf("version ordering verified across %d pairs", len(pairs))
}

func TestTypeRefString(t *testing.T) {
	tr := TypeRef{Array: &TypeRef{MapValue: &TypeRef{Primitive: PrimString}}}
	if got := tr.String(); got != "[]map[string]string" {
		t.Fatalf("got %q", got)
	}
	nullable := TypeRef{Primitive: PrimBool, Nullable: true}
	if got := nullable.String(); got != "bool?" {
		t.Fatalf("got %q", got)
	}
}
