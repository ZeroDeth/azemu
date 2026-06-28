//go:build !windows

package main

import "syscall"

// execProcess replaces the current process image with the named binary, the
// classic exec(3) behaviour. On success it never returns.
func execProcess(bin string, argv, env []string) error {
	return syscall.Exec(bin, argv, env)
}

// detachSysProcAttr starts the child in its own session (setsid) so it
// survives the parent's exit and detaches from the controlling terminal.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
