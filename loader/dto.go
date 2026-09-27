package loader

// fileDTO is the on-disk wire format of one interface description.
type fileDTO struct {
	Name       string         `json:"name"`
	Version    string         `json:"version"`
	Codec      string         `json:"codec"`
	Doc        string         `json:"doc"`
	Operations []operationDTO `json:"operations"`
	Types      []namedTypeDTO `json:"types"`
}

type operationDTO struct {
	Name   string  `json:"name"`
	Doc    string  `json:"doc"`
	Input  typeDTO `json:"input"`
	Output typeDTO `json:"output"`
}

type namedTypeDTO struct {
	Name string  `json:"name"`
	Doc  string  `json:"doc"`
	Type typeDTO `json:"type"`
}

// typeDTO is the recursive JSON encoding of schema.Type.
type typeDTO struct {
	Kind   string     `json:"kind"`
	Ref    string     `json:"ref,omitempty"`
	Elem   *typeDTO   `json:"elem,omitempty"`
	Values []string   `json:"values,omitempty"`
	Fields []fieldDTO `json:"fields,omitempty"`
}

type fieldDTO struct {
	Name     string  `json:"name"`
	Optional bool    `json:"optional,omitempty"`
	Doc      string  `json:"doc,omitempty"`
	Type     typeDTO `json:"type"`
}
