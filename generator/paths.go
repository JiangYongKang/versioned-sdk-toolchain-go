package generator

// Stable output-layout constants. Everything the toolchain writes lives
// under these names so repeated generations never leave stray files.
const (
	// sdkDirName holds manifests, history and the advisory lock.
	sdkDirName = ".sdkgen"
	// manifestName is the manifest of the current published generation.
	manifestName = "manifest.json"
	// historyName accumulates every accepted interface version.
	historyName = "history.json"
	// stagesDir is the scratch area for atomic publish; removed on commit.
	stagesDir = ".staging"
	// metadataDir holds machine-readable call metadata per service.
	metadataDir = "metadata"
	// goSDKDir holds generated Go client packages.
	goSDKDir = "go"
	// defaultModulePath is used when the caller does not specify one.
	defaultModulePath = "sdk-generated"
)
