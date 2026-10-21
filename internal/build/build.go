package build

// Version is the release version. GoReleaser overrides it through
// -ldflags "-X github.com/dscafati/secretref/internal/build.Version=...".
// It is a package-level variable because the linker can only set
// package-level strings. ADR 0015 lists the package-level values allowed.
var Version = "dev" //nolint:gochecknoglobals // set by ldflags
