package generator

import (
	"bytes"
	"encoding/json"
)

// stableJSON deterministically marshals a value: keys are sorted and HTML
// escaping is disabled. Every generated artifact goes through this so
// repeated runs over identical input produce byte-identical output.
func stableJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var c bytes.Buffer
	if err := json.Compact(&c, raw); err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, c.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	pretty.WriteByte('\n')
	return pretty.Bytes(), nil
}
