# Crun Cgroup Path Fix and CRI-O Integration

<!-- toc -->
- [Overview](#overview)
- [Cgroup Path Detection Logic](#cgroup-path-detection-logic)
  - [Path Classification Rules](#path-classification-rules)
  - [Where Detection Happens](#where-detection-happens)
  - [Examples](#examples)
- [How CRI-O Talks to Crun](#how-cri-o-talks-to-crun)
  - [Architecture](#architecture)
  - [Container Creation: CRI-O to Conmon to Crun](#container-creation-cri-o-to-conmon-to-crun)
  - [Direct Crun Invocations](#direct-crun-invocations)
  - [The OCI Spec: config.json](#the-oci-spec-configjson)
  - [How CRI-O Constructs cgroupsPath](#how-cri-o-constructs-cgroupspath)
  - [The systemd-cgroup Flag Flow](#the-systemd-cgroup-flag-flow)
  - [End-to-End Flow Diagram](#end-to-end-flow-diagram)
<!-- /toc -->

## Overview

This document covers four related topics:

1. A fix in crun to handle explicit filesystem-style cgroup paths when the
   systemd cgroup manager is enabled, required for placing pods under a
   parent cgroup hierarchy.
2. The detection logic crun uses to distinguish systemd-style cgroup paths
   from filesystem-style paths.
3. How crun uses eBPF programs for cgroup v2 device access control.
4. The communication path from CRI-O through conmon to crun, and how
   cgroup configuration flows through the OCI spec.

---

## The Sub-Pod Cgroup Path Fix

### Problem

When CRI-O manages sub-pods (pods nested under a parent pod's cgroup
hierarchy), it constructs an explicit filesystem-style cgroup path like:

```
kubepods/burstable/pod-abc123/subpods/pod-def456/crio-container789
```

This path contains `/` separators and represents a direct location in the
cgroup filesystem. When crun is configured with `--systemd-cgroup`, it
attempts to pass this path to systemd as a transient unit name. Systemd
cannot handle arbitrary slash-delimited paths -- it expects a "systemd
triple" format (`slice:scope-prefix:name` containing `:`). The result is
that the container's cgroup is either created in the wrong location or the
creation fails entirely.

### Root Cause

Crun's cgroup manager selection was binary: if `--systemd-cgroup` was set,
always use the systemd manager. There was no inspection of the actual
`linux.cgroupsPath` value from the OCI spec to determine whether systemd
could handle it.

### The Fix (Commit `7ea7ade4`)

The fix introduces **automatic cgroup manager fallback**: when
`--systemd-cgroup` is enabled but the OCI spec contains a filesystem-style
cgroup path, crun silently switches to the cgroupfs manager for that
container. This ensures the cgroup is created directly at the correct
filesystem location.

The fix has four parts:

**Part 1 -- Cgroup manager auto-detection** (`src/libcrun/cgroup.c`):
A new function `force_cgroupfs_for_explicit_path()` inspects the cgroup
path and overrides the manager selection when the path is filesystem-style.
Called in both `libcrun_cgroup_preenter()` and `libcrun_cgroup_enter()`.

**Part 2 -- Container create path** (`src/libcrun/container.c`):
`setup_cgroup_manager()` applies the same detection before choosing the
cgroup manager, so the manager is correct from the start.

**Part 3 -- Container restore path** (`src/libcrun/container.c`):
`libcrun_container_restore()` applies the same logic so checkpoint/restore
also uses the correct manager.

**Part 4 -- BPF pin directory creation** (`src/libcrun/cgroup-systemd.c`):
When using path-based cgroups, the BPF pin path can be deeply nested.
`add_bpf_program()` now creates intermediate parent directories before
pinning.

**Part 5 -- Status reporting** (`src/libcrun/cgroup.c`):
`libcrun_cgroup_get_status()` now exposes `systemd_cgroup` in the status
so callers know which manager was actually used.

### Files Changed

| File                           | Change                                                                                                       |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------ |
| `src/libcrun/cgroup.c`         | Added `force_cgroupfs_for_explicit_path()`, called in preenter and enter; exposed `systemd_cgroup` in status |
| `src/libcrun/container.c`      | Applied path detection in `setup_cgroup_manager()` and restore path                                          |
| `src/libcrun/cgroup-systemd.c` | Added parent directory creation before BPF pin                                                               |

---

## Cgroup Path Detection Logic

The core of the fix is a heuristic that classifies cgroup paths into two
categories based on their syntax.

### Path Classification Rules

```
                        ┌──────────────────┐
                        │  cgroupsPath     │
                        │  from OCI spec   │
                        └────────┬─────────┘
                                 │
                        ┌────────▼─────────┐
                        │  Is it empty     │──── Yes ──→ Use configured
                        │  or NULL?        │              manager (default)
                        └────────┬─────────┘
                                 │ No
                        ┌────────▼─────────┐
                        │  Contains ':'?   │──── Yes ──→ Systemd triple
                        │                  │              (use systemd)
                        └────────┬─────────┘
                                 │ No
                        ┌────────▼─────────┐
                        │  Contains '/'?   │──── No ───→ Simple name
                        │                  │              (use systemd)
                        └────────┬─────────┘
                                 │ Yes
                        ┌────────▼─────────┐
                        │  Filesystem path │
                        │  FORCE CGROUPFS  │
                        └──────────────────┘
```

The detection function:

```c
static void
force_cgroupfs_for_explicit_path (struct libcrun_cgroup_args *args)
{
  const char *cp = args->cgroup_path;

  if (args->manager != CGROUP_MANAGER_SYSTEMD)
    return;                                     /* not using systemd, nothing to do */
  if (cp == NULL || cp[0] == '\0')
    return;                                     /* empty path, use configured manager */
  if (strchr (cp, '/') == NULL || strchr (cp, ':') != NULL)
    return;                                     /* no '/' or has ':', systemd can handle it */
  args->manager = CGROUP_MANAGER_CGROUPFS;      /* filesystem path, force cgroupfs */
}
```

### Where Detection Happens

The detection runs at three points to ensure consistent behavior:

| Location      | Function                      | Purpose                                           |
| ------------- | ----------------------------- | ------------------------------------------------- |
| `container.c` | `setup_cgroup_manager()`      | Initial manager selection during container create |
| `container.c` | `libcrun_container_restore()` | Manager selection during checkpoint restore       |
| `cgroup.c`    | `libcrun_cgroup_preenter()`   | Before cgroup pre-entry (unified mode only)       |
| `cgroup.c`    | `libcrun_cgroup_enter()`      | Before cgroup entry                               |

### Examples

| `linux.cgroupsPath` value             | Contains `/` | Contains `:` | Result                         |
| ------------------------------------- | :----------: | :----------: | ------------------------------ |
| `""` (empty)                          |      --      |      --      | Use configured manager         |
| `system.slice:crio:abc123`            |      No      |     Yes      | **Systemd** (valid triple)     |
| `my-container`                        |      No      |      No      | **Systemd** (simple name)      |
| `kubepods/burstable/pod-xyz/crio-abc` |     Yes      |      No      | **Cgroupfs** (filesystem path) |
| `kubepods.slice:crio:abc123`          |      No      |     Yes      | **Systemd** (triple with dots) |

---

## How Crun Uses BPF for Device Access Control

### Why BPF Is Needed

On cgroup v2 (unified hierarchy), the legacy `devices.allow` and
`devices.deny` files from cgroup v1 do not exist. Instead, device access
control is enforced via **eBPF programs** of type
`BPF_PROG_TYPE_CGROUP_DEVICE` attached to a cgroup with attach type
`BPF_CGROUP_DEVICE`. The kernel calls these programs on every device
access attempt, and the program returns allow (1) or deny (0).

### BPF Program Construction

Crun translates OCI device rules into eBPF bytecode through a builder
pattern:

**Step 1 -- Initialize** (`bpf_program_init_dev`): Emits a prologue that
loads the device access request context into BPF registers:

- R2 = device type (block/char)
- R3 = access flags (read/write/mknod)
- R4 = major number
- R5 = minor number

**Step 2 -- Append rules** (`bpf_program_append_dev`): For each OCI
device rule, emits compare-and-jump instructions matching type, access
mask, major, and minor. Each rule either accepts (return 1) or rejects
(return 0). Default devices (null, zero, full, random, urandom, tty,
ptmx, etc.) are always prepended.

**Step 3 -- Complete** (`bpf_program_complete_dev`): Appends a final
deny-all instruction (return 0) unless a wildcard rule was encountered.

The entry point is `create_dev_bpf()` in `src/libcrun/cgroup-resources.c`:

```c
struct bpf_program *
create_dev_bpf (runtime_spec_schema_defs_linux_device_cgroup **devs,
                size_t devs_len, libcrun_error_t *err)
```

### Loading and Attaching

`libcrun_ebpf_load()` in `src/libcrun/ebpf.c` handles both loading and
attaching:

1. **`BPF_PROG_LOAD`** -- Loads the bytecode into the kernel with program
   type `BPF_PROG_TYPE_CGROUP_DEVICE` and license `"GPL"`. Retries with
   `RLIMIT_MEMLOCK` bump if needed.

2. **Attach to cgroup** (when `dirfd >= 0`): Calls `ebpf_attach_program()`
   which:
   - Queries existing attached programs via `BPF_PROG_QUERY`.
   - Attaches the new program with `BPF_PROG_ATTACH` and
     `BPF_F_ALLOW_MULTI`.
   - If exactly one program was already attached and the kernel supports
     it, uses `BPF_F_REPLACE` for atomic replacement.
   - Otherwise, detaches old programs after the new one is attached.

3. **Pin to filesystem** (when `pin != NULL`): Calls `BPF_OBJ_PIN` to
   persist the program at a path under `/sys/fs/bpf/crun/`. This is used
   by the systemd manager so that systemd can load the pinned program.

### BPF Pin Path Construction

The pin path is constructed by `bpfprog_path_from_scope()` in
`src/libcrun/cgroup-systemd.c`:

```c
static int
bpfprog_path_from_scope (char **path, const char *scope, libcrun_error_t *err)
{
  cleanup_free char *flat_scope = xstrdup (scope);
  char *it;

  /* Remove dots as EBPF code don't like them. */
  it = flat_scope;
  while ((it = strchr (it, '.')) != NULL)
    *it = '_';

  return append_paths (path, err, CRUN_BPF_DIR, flat_scope, NULL);
}
```

This produces paths like:

```
/sys/fs/bpf/crun/crio-abc123_scope
```

Where `CRUN_BPF_DIR` is `/sys/fs/bpf/crun` (defined in
`src/libcrun/ebpf.h`).

### The Pin Directory Fix

With path-based cgroups (the fix described above), the scope can contain
slashes, producing deeply nested pin paths like:

```
/sys/fs/bpf/crun/kubepods/burstable/pod-xyz/crio-abc123
```

`BPF_OBJ_PIN` requires all parent directories to exist. Before the fix,
only `/sys/fs/bpf/crun` was created. The fix adds:

```c
{
  cleanup_free char *pin_parent = xstrdup (path);
  char *last_slash = strrchr (pin_parent, '/');

  if (last_slash != NULL && last_slash != pin_parent)
    {
      *last_slash = '\0';
      ret = crun_ensure_directory (pin_parent, 0700, false, err);
      if (UNLIKELY (ret < 0))
        return ret;
    }
}
```

This recursively creates intermediate directories so `BPF_OBJ_PIN`
succeeds for any nesting depth.

### Systemd vs Cgroupfs BPF Paths

| Manager      | How BPF is applied                                                                                                                                                                                                         |
| ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Systemd**  | `add_bpf_program()` creates the BPF program, pins it at `/sys/fs/bpf/crun/<flat-scope>`, then sets the `BPFProgram` property on the systemd transient unit. Systemd loads from the pin and attaches to the scope's cgroup. |
| **Cgroupfs** | `write_devices_resources_v2_internal()` creates the BPF program and attaches it directly to the cgroup directory fd via `libcrun_ebpf_load(program, dirfd, NULL)`. No pin is needed.                                       |

When systemd has already set device rules via BPF (indicated by the
`bpf_dev_set` flag in `struct libcrun_cgroup_status`), crun skips applying
device rules again through the cgroupfs path to avoid conflicts.

### Updates and Comparison

When updating device rules on an existing container (systemd path):

1. Read the existing pinned program via `libcrun_ebpf_read_program()`
   (uses `BPF_OBJ_GET` + `BPF_OBJ_GET_INFO_BY_FD` to retrieve the
   translated bytecode).
2. Compare with the new program via `libcrun_ebpf_cmp_programs()` (byte
   comparison of instruction buffers).
3. If identical, skip the update.
4. If different, attempt a normalization check (load the new program,
   pin to a temp path, read it back, compare again) to handle
   architecture-specific bytecode differences from the verifier.

For the cgroupfs path, `ebpf_attach_program()` handles updates via
atomic `BPF_F_REPLACE` when possible.

### Cleanup

When a container is destroyed, `libcrun_destroy_cgroup_systemd()` calls
`unlink()` on the BPF pin path to remove the pinned program from
`/sys/fs/bpf/crun/`.

---

## How CRI-O Talks to Crun

### Architecture

CRI-O does not invoke crun directly for container creation. Instead, it
uses **conmon** (container monitor) as an intermediary. Conmon manages
the container's stdio, writes logs, and tracks exit status. For other
operations (exec, delete, state), CRI-O may invoke crun directly.

```
                    ┌──────────┐
                    │  CRI-O   │
                    │  Server  │
                    └────┬─────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
         create/start  exec     delete/state
              │          │          │
              ▼          │          ▼
        ┌──────────┐     │    ┌──────────┐
        │  conmon   │     │    │   crun   │
        │ (monitor) │     │    │ (direct) │
        └────┬─────┘     │    └──────────┘
             │           │
             ▼           ▼
        ┌──────────────────┐
        │       crun       │
        │  (OCI runtime)   │
        └──────────────────┘
```

### Container Creation: CRI-O to Conmon to Crun

CRI-O's `runtimeOCI.CreateContainer()` builds a conmon command line with
these key arguments:

| Flag                          | Purpose                                                  |
| ----------------------------- | -------------------------------------------------------- |
| `-b <bundle>`                 | OCI bundle directory (contains `config.json`)            |
| `-c <id>`                     | Container ID                                             |
| `-r <runtime>`                | Path to the OCI runtime binary (e.g. `/usr/bin/crun`)    |
| `--runtime-arg --root=<path>` | Runtime state root directory                             |
| `-s`                          | **Enable systemd cgroup mode** (only when `IsSystemd()`) |
| `-l <logpath>`                | Container log file path                                  |
| `--exit-dir <dir>`            | Directory where exit files are written                   |
| `-n <name>`                   | Container name                                           |
| `-P <pidfile>`                | Conmon's own PID file                                    |
| `-p <pidfile>`                | Container PID file (in bundle)                           |

The relevant CRI-O code in `internal/oci/runtime_oci.go`:

```go
args := []string{
    "-b", c.bundlePath,
    "-c", c.ID(),
    "--exit-dir", r.config.ContainerExitsDir,
    "-l", c.logPath,
    "-n", c.name,
    "-r", c.RuntimePathForPlatform(r),
    "--runtime-arg", fmt.Sprintf("%s=%s", rootFlag, r.root),
    "-u", c.ID(),
}

if r.config.CgroupManager().IsSystemd() {
    args = append(args, "-s")
}
```

The `-s` flag tells conmon to use systemd cgroup mode. Conmon then passes
the equivalent of `--systemd-cgroup` to crun when it exec's the runtime.

### Direct Crun Invocations

For operations that bypass conmon (e.g. `exec`, `delete`, `state`), CRI-O
calls crun directly with `defaultRuntimeArgs()`:

```go
func (r *runtimeOCI) defaultRuntimeArgs() []string {
    args := []string{rootFlag, r.root}
    if r.config.CgroupManager().IsSystemd() {
        args = append(args, "--systemd-cgroup")
    }
    return args
}
```

This produces commands like:

```bash
crun --root /run/crio/crun --systemd-cgroup exec <container-id> -- <cmd>
```

### The OCI Spec: config.json

The OCI runtime spec (`config.json`) is the primary interface between
CRI-O and crun for container configuration. CRI-O writes this file to
the bundle directory before invoking conmon/crun. Crun reads it via
`libcrun_container_load_from_file()`:

```c
container_def = runtime_spec_schema_config_schema_parse_file (path, NULL, &oci_error);
```

The cgroup path is at `linux.cgroupsPath` in the JSON, which maps to
`def->linux->cgroups_path` in crun's C struct. The cgroup resource limits
are at `linux.resources`.

### How CRI-O Constructs cgroupsPath

CRI-O constructs the `linux.cgroupsPath` value differently depending on
the cgroup manager:

**Systemd manager** (`internal/config/cgmgr/systemd_linux.go`):

The path is a systemd triple `slice:prefix:id`:

```go
func (*SystemdManager) ContainerCgroupPath(sbParent, containerID string) string {
    parent := defaultSystemdParent
    if sbParent != "" {
        parent = sbParent
    }
    return parent + ":" + CrioPrefix + ":" + containerID
}
```

Example: `kubepods-burstable-podXYZ.slice:crio:container-abc123`

**Cgroupfs manager** (`internal/config/cgmgr/cgroupfs_linux.go`):

The path is a filesystem path:

```go
func (*CgroupfsManager) ContainerCgroupPath(sbParent, containerID string) string {
    parent := defaultCgroupfsParent
    if sbParent != "" {
        parent = sbParent
    }
    return filepath.Join("/", parent, containerCgroupPath(containerID))
}
```

Example: `/kubepods/burstable/pod-xyz/crio-container-abc123`

**Sub-pods** (`internal/config/cgmgr/subpod_linux.go`):

For sub-pods, an OCI-relative path is constructed under the parent pod's
cgroup:

```go
func OCIRelativeCgroupPath(absPath string) (string, error) {
    rooted := cgroupV2FilesystemPath(absPath)
    rel, err := filepath.Rel(CgroupMemoryPathV2, rooted)
    // ...
    return rel, nil
}
```

Example: `kubepods/burstable/pod-parent/subpods/pod-child/crio-ctr`

This sub-pod path is what triggers the cgroup manager fallback fix in
crun -- it contains `/` but no `:`, so crun uses cgroupfs instead of
systemd to place the cgroup at the exact filesystem location CRI-O
expects.

### The systemd-cgroup Flag Flow

```
CRI-O config                 conmon                     crun
─────────────                ──────                     ────

cgroup_manager = "systemd"
        │
        ▼
IsSystemd() == true
        │
        ├──── Container Create ──→ conmon -s ──→ crun --systemd-cgroup
        │                                              │
        │                                              ▼
        │                                     context->systemd_cgroup = true
        │                                              │
        │                                              ▼
        │                                     Check linux.cgroupsPath:
        │                                       Has '/' and no ':' ?
        │                                              │
        │                                     ┌────────┴────────┐
        │                                   Yes                 No
        │                                     │                  │
        │                                     ▼                  ▼
        │                               Use CGROUPFS        Use SYSTEMD
        │                               (filesystem path)   (triple path)
        │
        └──── Direct exec/delete ─────→ crun --systemd-cgroup <cmd>
                                               │
                                               ▼
                                        (same detection logic)
```

### End-to-End Flow Diagram

This shows the complete path from kubelet to cgroup creation:

```
1. Kubelet ──CRI gRPC──→ CRI-O Server
                              │
2. CRI-O constructs          │  linux.cgroupsPath =
   cgroup path               │    "slice:crio:id"      (systemd)
                              │    "/parent/crio-id"    (cgroupfs)
                              │    "parent/sub/crio-id" (sub-pod)
                              │
3. CRI-O writes config.json  │
   to bundle directory        │
                              │
4. CRI-O spawns conmon        │  conmon -r /usr/bin/crun
                              │         -b /bundle -c <id> -s
                              │
5. Conmon execs crun           │  crun --systemd-cgroup create <id>
                              │
6. Crun reads config.json     │  def->linux->cgroups_path
                              │
7. Crun detects path type     │  Has '/' and no ':' ?
                              │     → force cgroupfs
                              │     → else use systemd
                              │
8. Crun creates cgroup         │  cgroupfs: mkdir under /sys/fs/cgroup/
                              │  systemd: create transient scope via D-Bus
                              │
9. Crun attaches BPF          │  device access control program
   (if cgroup v2)             │
                              │
10. Crun creates container    │  via OCI runtime create
```
