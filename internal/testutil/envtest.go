// Package testutil contains helpers shared by the test suites.
package testutil

import (
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// StopEnvTest stops the envtest control plane and waits until it is fully shut
// down. On Windows, controller-runtime's Stop() sends SIGTERM, which Go's os
// package does not support on Windows, so the signal call fails and etcd /
// kube-apiserver would otherwise be left running forever. The Windows
// implementation terminates those child processes before retrying Stop().
func StopEnvTest(env *envtest.Environment) error {
	return stopEnvTest(env)
}

// retryStop calls env.Stop() repeatedly until it succeeds. It is used on all
// platforms because Stop() may return before the child process has been fully
// reaped, in which case a follow-up call finishes the cleanup.
func retryStop(env *envtest.Environment, interval, timeout time.Duration) error {
	var lastErr error
	deadline := time.Now().Add(timeout)
	for {
		if err := env.Stop(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("stop envtest: %w", lastErr)
		}
		time.Sleep(interval)
	}
}
