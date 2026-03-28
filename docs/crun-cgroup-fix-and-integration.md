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
  - [Slice Name Encoding](#slice-name-encoding)
  - [How CRI-O Constructs Cgroup Paths](#how-cri-o-constructs-cgroup-paths)
  - [Directory Tree on Disk (Systemd + cgroup v2)](#directory-tree-on-disk-systemd--cgroup-v2)
  - [Directory Tree on Disk (Cgroupfs)](#directory-tree-on-disk-cgroupfs)
  - [The <code>container/</code> Sub-Cgroup (crun-specific)](#the-container-sub-cgroup-crun-specific)
  - [Conmon's Cgroup Placement](#conmons-cgroup-placement)
  - [Naming Convention Summary](#naming-convention-summary)
  <!-- /toc -->

## Overview

This document covers five related topics:

1. A fix in crun to handle explicit filesystem-style cgroup paths when the
   systemd cgroup manager is enabled, required for placing pods under a
   parent cgroup hierarchy.
2. The detection logic crun uses to distinguish systemd-style cgroup paths
   from filesystem-style paths.
3. How crun uses eBPF programs for cgroup v2 device access control.
4. The communication path from CRI-O through conmon to crun, and how
   cgroup configuration flows through the OCI spec.
5. Systemd cgroup concepts (`.slice`, `.scope`) and how the pod cgroup
   directory structure is organized on disk.

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

**The cgroup v1 approach (no BPF):**

On cgroup v1, device access control was handled by the kernel's built-in
**devices controller** via two pseudo-files:

- `devices.allow` -- whitelist a device (e.g. `c 1:3 rwm` for `/dev/null`)
- `devices.deny` -- blacklist a device (e.g. `a *:* rwm` to deny all)

The kernel maintained an internal allow/deny list and enforced it on
every `mknod`, `open`, or other device access syscall. This was simple
but inflexible -- the rule format was fixed, and the kernel had to
maintain per-cgroup linked lists that could not be extended with custom
logic.

**Why cgroup v2 dropped the devices controller:**

Cgroup v2 (unified hierarchy) removed the `devices.allow`/`devices.deny`
files entirely. The kernel developers chose to replace the built-in
device controller with a programmable eBPF-based mechanism for several
reasons:

1. **Flexibility** -- BPF programs can express complex access policies
   that the fixed allow/deny format could not (e.g. conditional rules
   based on device type AND access mode AND major/minor combinations in
   a single atomic program).

2. **Atomicity** -- A BPF program is loaded and attached as a single
   unit. With cgroup v1, writing multiple rules to `devices.allow` was
   not atomic -- there was a window between writes where the policy was
   incomplete. BPF eliminates this race.

3. **Performance** -- BPF programs run as JIT-compiled native code in
   the kernel. The v1 devices controller walked a linked list on every
   access check. BPF programs are verified at load time and execute as
   straight-line code with predictable performance.

4. **Hierarchical attachment** -- BPF programs can be attached at
   multiple levels of the cgroup hierarchy with `BPF_F_ALLOW_MULTI`.
   A parent cgroup's device policy is enforced alongside child policies,
   enabling layered security without duplicating rules.

5. **Atomic updates** -- The `BPF_F_REPLACE` flag allows atomically
   swapping an attached BPF program with a new one, so device policy
   updates never leave a window with no enforcement.

**What crun must do:**

Since cgroup v2 provides no file-based device control interface, crun
must translate the OCI spec's device rules (`linux.resources.devices`)
into a BPF program and attach it to the container's cgroup. This is not
optional -- without the BPF program, a cgroup v2 container would have
**unrestricted device access**.

The BPF program type is `BPF_PROG_TYPE_CGROUP_DEVICE`, attached with
type `BPF_CGROUP_DEVICE`. The kernel calls this program on every device
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

The `config.json` file (OCI runtime spec) is **created by CRI-O** and
**consumed by crun**. Crun never writes or modifies this file.

**CRI-O builds the spec in memory** using the container factory
(`internal/factory/container/`). During `CreateContainer`, it calls
methods like `SpecAddMount`, `SpecSetProcessArgs`, `SpecAddDevices`,
`SpecSetLinuxContainerResources`, and `SpecAddNamespaces` to assemble
the full spec from the CRI request and image configuration.

**CRI-O writes the spec to two locations** per container
(`server/container_create.go`):

```go
specgen.SaveToFile(filepath.Join(containerInfo.Dir, "config.json"), saveOptions)
specgen.SaveToFile(filepath.Join(containerInfo.RunDir, "config.json"), saveOptions)
```

| Location               | Purpose                                                           |
| ---------------------- | ----------------------------------------------------------------- |
| `containerInfo.Dir`    | Persistent directory -- survives reboots, used for state recovery |
| `containerInfo.RunDir` | Volatile runtime directory -- the OCI bundle that crun reads      |

The same dual-save pattern applies to sandbox infra containers in
`server/sandbox_run_linux.go`.

**Crun reads the spec** from the bundle directory (passed by conmon via
`-b`) using `libcrun_container_load_from_file()`:

```c
container_def = runtime_spec_schema_config_schema_parse_file (path, NULL, &oci_error);
```

| Step                | Component  | Action                                                                                |
| ------------------- | ---------- | ------------------------------------------------------------------------------------- |
| Build OCI spec      | **CRI-O**  | Container factory assembles mounts, devices, namespaces, resources, security profiles |
| Write `config.json` | **CRI-O**  | `specgen.SaveToFile()` to persistent and runtime directories                          |
| Pass bundle path    | **conmon** | `conmon -b /run/containers/storage/.../bundle`                                        |
| Parse `config.json` | **crun**   | `runtime_spec_schema_config_schema_parse_file()`                                      |
| Execute the spec    | **crun**   | Creates cgroups, namespaces, mounts, starts container process                         |

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

---

## Systemd Cgroup Concepts and Pod Directory Structure

### Systemd Unit Types for Cgroups

Systemd uses two unit types to organize cgroups:

**`.slice`** -- A grouping unit that creates a node in the cgroup
hierarchy. Slices do not run processes themselves; they exist to organize
other units into a tree. Slices can be nested, and a child slice's name
encodes the full hierarchy using `-` as a separator.

**`.scope`** -- An execution unit that wraps externally-started processes
(unlike `.service`, which systemd starts itself). CRI-O uses scopes for
containers because the container processes are started by crun/conmon, not
by systemd. Scopes live under slices.

| Unit type | Purpose                      | Example                     |
| --------- | ---------------------------- | --------------------------- |
| `.slice`  | Grouping / resource boundary | `kubepods-burstable.slice`  |
| `.scope`  | Wraps a running process tree | `crio-<container-id>.scope` |

### Slice Name Encoding

The `-` character in systemd slice names encodes hierarchy depth. Each
segment between dashes represents one level in the cgroup tree:

| Slice name                        | Meaning                         | Parent                     |
| --------------------------------- | ------------------------------- | -------------------------- |
| `kubepods.slice`                  | Top-level Kubernetes cgroup     | `-.slice` (root)           |
| `kubepods-burstable.slice`        | `burstable` child of `kubepods` | `kubepods.slice`           |
| `kubepods-burstable-podABC.slice` | Pod `ABC` under `burstable`     | `kubepods-burstable.slice` |

CRI-O (and the kubelet) use `systemd.ExpandSlice()` to convert a slice
name into its filesystem path:

| Slice name                        | `ExpandSlice` result                                                       |
| --------------------------------- | -------------------------------------------------------------------------- |
| `kubepods.slice`                  | `/kubepods.slice`                                                          |
| `kubepods-burstable.slice`        | `/kubepods.slice/kubepods-burstable.slice`                                 |
| `kubepods-burstable-podABC.slice` | `/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podABC.slice` |

### How CRI-O Constructs Cgroup Paths

CRI-O uses the `CrioPrefix` constant (`"crio"`) defined in
`internal/config/cgmgr/cgmgr_linux.go` to build container cgroup names:

```go
func containerCgroupPath(id string) string {
    return CrioPrefix + "-" + id    // "crio-<id>"
}
```

**Systemd manager** -- The OCI spec `linux.cgroupsPath` is set to a
systemd triple `slice:prefix:id`:

```go
// ContainerCgroupPath returns e.g.:
// "kubepods-burstable-podABC.slice:crio:container-123"
func (*SystemdManager) ContainerCgroupPath(sbParent, containerID string) string {
    return parent + ":" + CrioPrefix + ":" + containerID
}
```

To get the absolute path on disk, CRI-O expands the slice and appends
the scope name (`internal/config/cgmgr/systemd_linux.go`):

```go
func (m *SystemdManager) ContainerCgroupAbsolutePath(sbParent, containerID string) (string, error) {
    cgroup, err := systemd.ExpandSlice(parent)
    // ...
    return filepath.Join(cgroup, containerCgroupPath(containerID)+".scope"), nil
}
```

For example, with `sbParent = "kubepods-burstable-podABC.slice"` and
`containerID = "def456"`:

- Triple: `kubepods-burstable-podABC.slice:crio:def456`
- Absolute path: `/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podABC.slice/crio-def456.scope`

**Cgroupfs manager** -- Paths are simple filesystem paths without `.slice`
or `.scope` suffixes:

```go
func (*CgroupfsManager) ContainerCgroupPath(sbParent, containerID string) string {
    return filepath.Join("/", parent, containerCgroupPath(containerID))
}
// Result: "/kubepods/burstable/pod-ABC/crio-def456"
```

### Directory Tree on Disk (Systemd + cgroup v2)

```
/sys/fs/cgroup/
└── kubepods.slice/                                        ← kubelet top-level slice
    ├── kubepods-besteffort.slice/                          ← BestEffort QoS class
    │   └── kubepods-besteffort-pod<UID>.slice/             ← pod slice
    │       ├── crio-<INFRA_ID>.scope/                     ← infra (pause) container
    │       │   └── container/                             ← crun child cgroup
    │       ├── crio-<CTR_1>.scope/                        ← workload container 1
    │       │   └── container/
    │       └── crio-<CTR_2>.scope/                        ← workload container 2
    │           └── container/
    ├── kubepods-burstable.slice/                           ← Burstable QoS class
    │   └── kubepods-burstable-pod<UID>.slice/
    │       ├── crio-<INFRA_ID>.scope/
    │       │   └── container/
    │       └── crio-<CTR_ID>.scope/
    │           └── container/
    └── kubepods-pod<UID>.slice/                            ← Guaranteed QoS (no QoS sub-slice)
        ├── crio-<INFRA_ID>.scope/
        │   └── container/
        └── crio-<CTR_ID>.scope/
            └── container/
```

| Directory                           | Created by               | Type      | Purpose                                       |
| ----------------------------------- | ------------------------ | --------- | --------------------------------------------- |
| `kubepods.slice`                    | Kubelet                  | slice     | Top-level Kubernetes resource boundary        |
| `kubepods-burstable.slice`          | Kubelet                  | slice     | QoS class grouping                            |
| `kubepods-burstable-pod<UID>.slice` | Kubelet                  | slice     | Per-pod resource limits (CPU, memory)         |
| `crio-<ID>.scope`                   | crun (via systemd D-Bus) | scope     | Per-container cgroup                          |
| `container/`                        | crun                     | directory | Child cgroup for the actual container process |

### Directory Tree on Disk (Cgroupfs)

```
/sys/fs/cgroup/
└── kubepods/                                              ← kubelet top-level
    ├── besteffort/                                        ← BestEffort QoS class
    │   └── pod-<UID>/                                     ← pod directory
    │       ├── crio-<INFRA_ID>/                           ← infra container
    │       └── crio-<CTR_ID>/                             ← workload container
    ├── burstable/                                         ← Burstable QoS class
    │   └── pod-<UID>/
    │       └── crio-<CTR_ID>/
    └── pod-<UID>/                                         ← Guaranteed QoS
        └── crio-<CTR_ID>/
```

### The `container/` Sub-Cgroup (crun-specific)

Crun creates an additional `container/` child directory inside each
scope. This exists because systemd enforces a **single-owner rule**: only
the unit manager (systemd) should write to cgroup control files in a scope
it manages. By creating a child cgroup and placing the container process
there, crun avoids conflicting with systemd's cgroup management.

CRI-O accounts for this in `crunContainerCgroupManager()`
(`internal/config/cgmgr/cgmgr_linux.go`):

```go
func crunContainerCgroupManager(expectedContainerCgroup string) (cgroups.Manager, error) {
    actualContainerCgroup := filepath.Join(expectedContainerCgroup, "container")
    // Check if the "container" child cgroup exists
    // If it does, return a cgroup manager for it
    // ...
}
```

This is relevant for stats collection: CRI-O must read resource usage from
the `container/` child (where the process actually runs), not from the
parent scope (which systemd owns). The `PodAndContainerCgroupManagers()`
method returns managers for both levels.

### Conmon's Cgroup Placement

Conmon (the container monitor) is placed in a separate scope under the
same pod slice. CRI-O creates a transient systemd scope named
`crio-conmon-<container-id>.scope` via D-Bus:

```go
conmonUnitName := fmt.Sprintf("crio-conmon-%s.scope", cid)
```

This keeps conmon's resource usage accounted separately from the container
it monitors, while still under the pod's resource limits.

### Naming Convention Summary

| Component            | Systemd name                        | On-disk path (under `/sys/fs/cgroup/`)                          |
| -------------------- | ----------------------------------- | --------------------------------------------------------------- |
| K8s top-level        | `kubepods.slice`                    | `kubepods.slice/`                                               |
| QoS class            | `kubepods-burstable.slice`          | `kubepods.slice/kubepods-burstable.slice/`                      |
| Pod                  | `kubepods-burstable-pod<UID>.slice` | `.../kubepods-burstable-pod<UID>.slice/`                        |
| Container (OCI spec) | `<pod-slice>:crio:<ID>`             | -- (triple, not a path)                                         |
| Container (on disk)  | `crio-<ID>.scope`                   | `.../kubepods-burstable-pod<UID>.slice/crio-<ID>.scope/`        |
| crun child           | --                                  | `.../crio-<ID>.scope/container/`                                |
| Conmon               | `crio-conmon-<ID>.scope`            | `.../kubepods-burstable-pod<UID>.slice/crio-conmon-<ID>.scope/` |
