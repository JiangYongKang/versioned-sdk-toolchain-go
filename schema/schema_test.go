package schema

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"v1", "v1.0.0", false},
		{"1", "v1.0.0", false},
		{"V1.2", "v1.2.0", false},
		{"v1.2.3", "v1.2.3", false},
		{"0.0.0", "v0.0.0", false},
		{"", "", true},
		{"v", "", true},
		{"v-1", "", true},
		{"v1.2.3.4", "", true},
		{"v1..0", "", true},
		{"latest", "", true},
		{"v1-alpha", "", true},
	}
	for _, tc := range cases {
		got, err := ParseVersion(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseVersion(%q): expected error, got %s", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got.String() != tc.want {
			t.Errorf("ParseVersion(%q): got %s want %s", tc.in, got, tc.want)
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
	if v("v1.9.9").Compare(v("v2.0.0")) >= 0 {
		t.Fatal("1.9.9 should be < 2.0.0")
	}
	if v("v2.0.1").Compare(v("v2.0.0")) <= 0 {
		t.Fatal("2.0.1 should be > 2.0.0")
	}
	if v("v2.0.0").Compare(v("v2")) != 0 {
		t.Fatal("v2 and v2.0.0 must compare equal")
	}
}
