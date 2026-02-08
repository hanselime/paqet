package version

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Version   = "v1.0.0-alpha.15"
	GitCommit = "unknown"
	GitTag    = "unknown"
	BuildTime = "unknown"
	GoVersion = runtime.Version()
)

var Cmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version:    %s\n", Version)
		fmt.Printf("Git Tag:    %s\n", GitTag)
		fmt.Printf("Git Commit: %s\n", GitCommit)
		fmt.Printf("Build Time: %s\n", BuildTime)
		fmt.Printf("Go Version: %s\n", GoVersion)
		fmt.Printf("Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
