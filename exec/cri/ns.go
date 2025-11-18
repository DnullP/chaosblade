//go:build linux
// +build linux

package cri

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"
)

// runInNamespaces locks the current OS thread, enters the provided namespaces of targetPid,
// executes the callback, and returns any error. Caller must ensure privileges are sufficient.
func runInNamespaces(targetPid int, namespaces []string, f func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// open and set ns for each requested namespace
	// Note: we don't attempt to restore original namespaces here; the goroutine will exit
	// after f returns, so thread teardown is acceptable for PoC.
	fhs := make([]*os.File, 0, len(namespaces))
	for _, ns := range namespaces {
		nsPath := filepath.Join("/proc", fmt.Sprintf("%d", targetPid), "ns", ns)
		fh, err := os.Open(nsPath)
		if err != nil {
			// close any opened fhs
			for _, of := range fhs {
				of.Close()
			}
			return fmt.Errorf("open ns %s failed: %v", nsPath, err)
		}
		if err := unix.Setns(int(fh.Fd()), 0); err != nil {
			fh.Close()
			for _, of := range fhs {
				of.Close()
			}
			return fmt.Errorf("setns %s failed: %v", nsPath, err)
		}
		fhs = append(fhs, fh)
	}

	// execute callback inside the namespaces on this thread
	err := f()

	// cleanup fhs
	for _, of := range fhs {
		of.Close()
	}
	return err
}
