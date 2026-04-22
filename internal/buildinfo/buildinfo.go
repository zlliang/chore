package buildinfo

// Version is the application version, overridable via ldflags:
//
//	go build -ldflags "-X github.com/zlliang/chore/internal/buildinfo.Version=1.0.0"
var Version = "dev"
