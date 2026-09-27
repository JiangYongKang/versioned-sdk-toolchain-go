package codegen

import "sort"

func sortMeta(md *Metadata) {
	sort.Slice(md.Interfaces, func(i, j int) bool { return md.Interfaces[i].Name < md.Interfaces[j].Name })
	sort.Slice(md.Enums, func(i, j int) bool { return md.Enums[i].Name < md.Enums[j].Name })
	sort.Slice(md.Structs, func(i, j int) bool { return md.Structs[i].Name < md.Structs[j].Name })
	sort.Strings(md.Artifacts)
}
