//go:build !windows

package testutil

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func stopEnvTest(env *envtest.Environment) error {
	return retryStop(env, 500*time.Millisecond, time.Minute)
}

// KillOrphanedEnvTestProcesses is a no-op on non-Windows platforms.
func KillOrphanedEnvTestProcesses(assetDirs ...string) {
	_ = assetDirs
}
