//go:build linux

package ipfetcher

import "syscall"

func bindSocketToDevice(fd uintptr, ifaceName string) error {
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, ifaceName)
}
