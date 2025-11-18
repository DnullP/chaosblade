//go:build !linux
package cri

import "fmt"

// runInNamespaces is only implemented on Linux (uses setns).
// Provide a stub on non-Linux platforms to avoid build errors.
func runInNamespaces(targetPid int, namespaces []string, f func() error) error {
    return fmt.Errorf("runInNamespaces is only supported on linux; targetPid=%d", targetPid)
}
