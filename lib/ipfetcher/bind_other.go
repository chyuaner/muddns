//go:build !linux

package ipfetcher

func bindSocketToDevice(fd uintptr, ifaceName string) error {
	return nil
}
