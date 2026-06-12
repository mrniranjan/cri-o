//go:build linux

package cgmgr

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cri-o/cri-o/internal/lib/stats"
)

// ContainerCgroupDirName returns the leaf cgroup directory name for a container id (crio-<id>).
func ContainerCgroupDirName(containerID string) string {
	return containerCgroupPath(containerID)
}

// cgroupV2FilesystemPath returns absPath as a path under the cgroup v2 unified mount.
// Systemd cgroup paths from ExpandSlice are hierarchy paths rooted at the slice (for example
// /kubepods.slice/...) and omit the /sys/fs/cgroup prefix; OCI and filepath.Rel need the full path.
func cgroupV2FilesystemPath(absPath string) string {
	if strings.HasPrefix(absPath, CgroupMemoryPathV2+"/") || absPath == CgroupMemoryPathV2 {
		return absPath
	}

	return filepath.Join(CgroupMemoryPathV2, strings.TrimPrefix(absPath, "/"))
}

// OCIRelativeCgroupPath returns the linux.cgroupsPath value relative to the cgroup v2 mount root.
func OCIRelativeCgroupPath(absPath string) (string, error) {
	rooted := cgroupV2FilesystemPath(absPath)

	rel, err := filepath.Rel(CgroupMemoryPathV2, rooted)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("cgroup path %q is not under %s", absPath, CgroupMemoryPathV2)
	}

	return rel, nil
}

// SubpodInfraCgroupAbsPath returns the absolute path to the infra cgroup directory for a sub-pod.
func SubpodInfraCgroupAbsPath(subpodBaseAbs, sandboxID string) string {
	return filepath.Join(subpodBaseAbs, containerCgroupPath(sandboxID))
}

// ContainerCgroupPathForSubpodWorkload returns the OCI-relative cgroupsPath for a workload under subpodBaseAbs.
func ContainerCgroupPathForSubpodWorkload(subpodBaseAbs, containerID string) (string, error) {
	abs := filepath.Join(subpodBaseAbs, containerCgroupPath(containerID))

	return OCIRelativeCgroupPath(abs)
}

// EnsureSubpodSandboxCgroups creates parent/subpods/<childSandboxID>/crio-<sbID> under parentScopeAbs.
func EnsureSubpodSandboxCgroups(parentScopeAbs, childSandboxID, sbID string) error {
	if err := createSandboxCgroup(parentScopeAbs, "subpods"); err != nil {
		return fmt.Errorf("create subpods cgroup under %s: %w", parentScopeAbs, err)
	}

	subpodsDir := filepath.Join(parentScopeAbs, "subpods")

	if err := createSandboxCgroup(subpodsDir, childSandboxID); err != nil {
		return fmt.Errorf("create subpod directory for %s: %w", childSandboxID, err)
	}

	leafParent := filepath.Join(subpodsDir, childSandboxID)

	if err := createSandboxCgroup(leafParent, containerCgroupPath(sbID)); err != nil {
		return fmt.Errorf("create subpod infra cgroup: %w", err)
	}

	return nil
}

func cgroupStatsUnderBase(subpodBaseAbs, containerID string) (*stats.CgroupStats, error) {
	cgMgr, err := LibctrManager(containerCgroupPath(containerID), subpodBaseAbs, false)
	if err != nil {
		return nil, err
	}

	return statsFromLibctrMgr(cgMgr)
}


// removeSubpodCgroupTree removes the infra cgroup and the subpods/<child> directory.
func removeSubpodCgroupTree(subpodBaseAbs, sbID string) error {
	if err := removeSandboxCgroup(subpodBaseAbs, containerCgroupPath(sbID)); err != nil {
		return err
	}

	return removeSandboxCgroup(filepath.Dir(subpodBaseAbs), filepath.Base(subpodBaseAbs))
}
