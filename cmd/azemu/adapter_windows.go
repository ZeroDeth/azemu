//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// execProcess emulates Unix exec on Windows, which has no execve. It runs the
// binary as a child inheriting stdio, waits for it, and exits with the child's
// exit code so the wrapper is transparent to the caller.
func execProcess(bin string, argv, env []string) error {
	cmd := exec.Command(bin, argv[1:]...) //nolint:gosec // bin is resolved via exec.LookPath
	cmd.Args = argv
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}

// detachSysProcAttr starts the child in a new process group so it is not killed
// when the parent's console closes. Windows has no setsid equivalent.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}
