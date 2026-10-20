package build

// Version is the release version. GoReleaser overrides it through
// -ldflags "-X github.com/dscafati/secretref/internal/build.Version=...".
// It is the only package-level variable allowed in the module, because the
// linker can only set package-level strings.
var Version = "dev" //nolint:gochecknoglobals // set by ldflags
