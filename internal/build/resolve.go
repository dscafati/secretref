package build

import "runtime/debug"

// devVersion is what Version holds when the linker did not set it.
const devVersion = "dev"

// Resolve returns the version to report. Version wins when the linker set it.
// Otherwise it is the main module version from readBuildInfo, which go
// install records (for example v0.1.0), unless that is empty or "(devel)".
// Otherwise it is "dev". main passes debug.ReadBuildInfo; a nil readBuildInfo
// means no build information.
func Resolve(readBuildInfo func() (*debug.BuildInfo, bool)) string {
	return resolve(Version, readBuildInfo)
}

func resolve(linked string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if linked != devVersion {
		return linked
	}
	if readBuildInfo == nil {
		return devVersion
	}
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return devVersion
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return devVersion
}
