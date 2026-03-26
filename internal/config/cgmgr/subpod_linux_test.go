//go:build linux

package cgmgr

import (
	"path/filepath"
	"testing"
)

func TestOCIRelativeCgroupPath(t *testing.T) {
	t.Parallel()

	rel, err := OCIRelativeCgroupPath(filepath.Join(CgroupMemoryPathV2, "kubepods.slice", "x.slice", "crio-abc", "subpods", "child", "crio-child"))
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join("kubepods.slice", "x.slice", "crio-abc", "subpods", "child", "crio-child")
	if rel != want {
		t.Fatalf("got %q want %q", rel, want)
	}
}

func TestContainerCgroupDirName(t *testing.T) {
	t.Parallel()

	if got, want := ContainerCgroupDirName("deadbeef"), CrioPrefix+"-deadbeef"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
