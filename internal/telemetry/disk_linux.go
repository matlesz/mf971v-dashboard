//go:build linux

package telemetry

import "syscall"

// collectDisks reports free space for the given mounts using statfs.
func collectDisks(mounts []string, warnings *[]string) []Disk {
	var out []Disk
	for _, mount := range mounts {
		var st syscall.Statfs_t
		if err := syscall.Statfs(mount, &st); err != nil {
			continue // mounts vary by firmware; absence is not an error
		}
		if st.Blocks == 0 || st.Bsize <= 0 {
			continue
		}
		blockSize := int64(st.Bsize)
		total := int64(st.Blocks) * blockSize / 1024
		free := int64(st.Bavail) * blockSize / 1024
		d := Disk{Mount: mount, TotalKB: total, FreeKB: free}
		if total > 0 {
			used := int(100 * (total - free) / total)
			d.UsedPct = &used
		}
		out = append(out, d)
	}
	if len(out) == 0 {
		*warnings = append(*warnings, "disks: statfs returned no usable mounts")
	}
	return out
}
