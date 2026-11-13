//go:build !linux

package telemetry

// collectDisks is a development-only stub so the package builds and tests on a
// workstation. Releases are always built for linux/arm.
func collectDisks(mounts []string, warnings *[]string) []Disk {
	*warnings = append(*warnings, "disks: unsupported on this build platform")
	return nil
}
