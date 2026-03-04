# CRI-O Data Structures Reference

<!-- toc -->

- [StreamService](#streamservice)
- [Container Types](#container-types)
  - [Container](#container)
  - [ContainerVolume](#containervolume)
  - [Container State Constants](#container-state-constants)
  - [ExecStarter](#execstarter)
  - [Builder Interface](#builder-interface)
- [Storage Types](#storage-types)
  - [ContainerInfo](#containerinfo)
  - [ImageResult](#imageresult)
  - [StorageImageID](#storageimageid)
  - [StorageTransport](#storagetransport)
  - [Seccomp Notifier and Notification](#seccomp-notifier-and-notification)
  - [CgroupManager Implementations](#cgroupmanager-implementations)
  - [ConmonManager](#conmonmanager)
  - [NamespaceManager](#namespacemanager)
  - [PodNamespacesConfig](#podnamespacesconfig)
  - [AppArmor Config](#apparmor-config)
  - [Capabilities](#capabilities)
  - [Ulimits Config](#ulimits-config)
  - [RuntimeImpl Interface](#runtimeimpl-interface)
- [Factory Types](#factory-types)
  - [Container Interface](#container-interface)
- [Hook Types](#hook-types)
  - [RuntimeHandlerHooks Interface](#runtimehandlerhooks-interface)
  - [HooksRetriever](#hooksretriever)
  - [PortMapping](#portmapping)
  - [SandboxMetrics](#sandboxmetrics)
  - [CgroupStats](#cgroupstats)
  - [Collector and Collectors](#collector-and-collectors)
  - [CommandRunner Interface](#commandrunner-interface)
  - [VersionInfo](#versioninfo)
  - [ExternalBindMount](#externalbindmount)
  - [Namespace Type Constants](#namespace-type-constants)
  - [Cgroup Constants](#cgroup-constants)
  <!-- /toc -->

See also: [API Reference](api-reference.md) |
[Container Creation Flow](container-creation-flow.md)

---

## Server Types

### Server

The central CRI-O server struct. Implements both `RuntimeServiceServer` and
`ImageServiceServer` gRPC interfaces.

```go
type Server struct {
    *lib.ContainerServer

    types.UnsafeImageServiceServer
    types.UnsafeRuntimeServiceServer

    config          libconfig.Config
    stream          *StreamService
    hostportManager hostport.HostPortManager
    monitorsChan    chan struct{}
    defaultIDMappings *idtools.IDMappings

    pullOperationsInProgress map[pullArguments]*pullOperation
    resourceStore            *resourcestore.ResourceStore
    seccompNotifierChan      chan seccomp.Notification
    nri                      *nriAPI
    hooksRetriever           *runtimehandlerhooks.HooksRetriever
    artifactStore            *ociartifact.Store
}
```

**Package:** `server`

**Embeds:** [`*lib.ContainerServer`](#containerserver) (provides access to
runtime, storage, and container/sandbox state)

**Consumed by:** `cmd/crio/main.go` for gRPC registration

**See also:** [API Reference -- Server Lifecycle](api-reference.md#server-lifecycle)

---

### StreamService

Implements the `streaming.Runtime` interface for Exec, Attach, and
PortForward streaming endpoints.

```go
type StreamService struct {
    streaming.Runtime
    ctx               context.Context
    runtimeServer     *Server
    streamServer      streaming.Server
    streamServerCloseCh chan struct{}
}
```

**Package:** `server`

---

### Inspect Endpoint Constants

```go
const InspectConfigEndpoint     = "/config"
const InspectContainersEndpoint = "/containers"
const InspectInfoEndpoint       = "/info"
const InspectPauseEndpoint      = "/pause"
const InspectUnpauseEndpoint    = "/unpause"
const InspectGoRoutinesEndpoint = "/debug/goroutines"
const InspectHeapEndpoint       = "/debug/heap"
```

**Package:** `server`

---

## Container Types

### Container

Represents a runtime container with all its metadata, state, and
configuration. This is the primary container object used throughout CRI-O.

```go
type Container struct {
    id                string
    name              string
    bundlePath        string
    logPath           string
    labels            map[string]string
    crioAnnotations   map[string]string
    annotations       map[string]string
    userRequestedImage string
    someNameOfTheImage *references.RegistryImageReference
    imageID           *storage.StorageImageID
    sandbox           string
    terminal          bool
    stdin             bool
    stdinOnce         bool
    runtimeHandler    string
    dir               string
    stopSignal        string
    created           time.Time
    state             *ContainerState
    volumes           []ContainerVolume
    mountPoint        string
    spec              *specs.Spec
    idMappings        *idtools.IDMappings
    // ... additional internal fields
}
```

**Package:** `internal/oci`

**Created by:** [`NewContainer()`](api-reference.md#newcontainer),
[`NewSpoofedContainer()`](api-reference.md#newspoofedcontainer)

**Used by:** `server`, `lib`, `factory/container`, `runtimehandlerhooks`,
`statsserver`

---

### ContainerState

Tracks the lifecycle state of a container.

```go
type ContainerState struct {
    specs.State

    Created                 time.Time
    Started                 time.Time
    Finished                time.Time
    ExitCode                *int32
    OOMKilled               bool
    SeccompKilled           bool
    Error                   string
    InitPid                 int
    InitStartTime           string
    CheckpointedAt          time.Time
    ContainerMonitorProcess *ContainerMonitorProcess
}
```

**Package:** `internal/oci`

**Embedded in:** [`Container`](#container) (as `state` field)

| Field                     | Description                                              |
| ------------------------- | -------------------------------------------------------- |
| `Created`                 | Timestamp when the container was created                 |
| `Started`                 | Timestamp when the container started running             |
| `Finished`                | Timestamp when the container exited                      |
| `ExitCode`                | Exit code (nil if still running)                         |
| `OOMKilled`               | Whether the container was killed by OOM                  |
| `SeccompKilled`           | Whether the container was killed by seccomp              |
| `Error`                   | Error message if the container failed                    |
| `InitPid`                 | PID of the container's init process                      |
| `InitStartTime`           | Start time of the init process (for PID reuse detection) |
| `CheckpointedAt`          | When the container was last checkpointed                 |
| `ContainerMonitorProcess` | conmon/conmon-rs process info                            |

---

### ContainerVolume

Represents a bind mount for a container.

```go
type ContainerVolume struct {
    ContainerPath     string
    HostPath          string
    Readonly          bool
    RecursiveReadOnly bool
    Propagation       types.MountPropagation
    SelinuxRelabel    bool
    Image             *types.ImageSpec
}
```

**Package:** `internal/oci`

---

### ContainerMonitorProcess

Tracks the container monitor process (conmon or conmon-rs).

```go
type ContainerMonitorProcess struct {
    Pid       int
    StartTime string
}
```

**Package:** `internal/oci`

---

### Container State Constants

```go
const (
    ContainerStateCreated = "created"
    ContainerStatePaused  = "paused"
    ContainerStateRunning = "running"
    ContainerStateStopped = "stopped"
)
```

**Package:** `internal/oci`

---

### ExecSyncError

Wraps command output and exit code for `ExecSync` failures.

```go
type ExecSyncError struct {
    Stdout   bytes.Buffer
    Stderr   bytes.Buffer
    ExitCode int32
    Err      error
}

func (e *ExecSyncError) Error() string
```

**Package:** `internal/oci`

---

### ExecStarter

Interface for starting an exec command and retrieving its PID.

```go
type ExecStarter interface {
    Start() error
    GetPid() int
}
```

**Package:** `internal/oci`

---

## Sandbox Types

### Sandbox

Represents a Kubernetes pod sandbox. Contains all pod-level metadata,
namespace information, and references to the pod's containers.

```go
type Sandbox struct {
    id               string
    namespace        string
    name             string
    kubeName         string
    logDir           string
    labels           fields.Set
    annotations      map[string]string
    containers       memorystore.Storer[*oci.Container]
    processLabel     string
    mountLabel       string
    metadata         *types.PodSandboxMetadata
    shmPath          string
    cgroupParent     string
    privileged       bool
    runtimeHandler   string
    resolvPath       string
    hostname         string
    portMappings     []*hostport.PortMapping
    hostNetwork      bool
    createdAt        time.Time
    infraContainer   *oci.Container
    stopped          bool
    created          bool
    networkStopped   bool
    namespaceOptions *types.NamespaceOption
    seccompProfilePath string
    usernsMode       string
    dnsConfig        *types.DNSConfig
    ips              []string
    podLinuxOverhead *types.LinuxContainerResources
    podLinuxResources *types.LinuxContainerResources
    managedNamespaces []nsmgr.Namespace
    criSandbox       *types.PodSandbox
    // ...
}
```

**Package:** `internal/lib/sandbox`

**Created by:** [`sandbox.NewBuilder()`](api-reference.md#newbuilder)

**Used by:** `server`, `lib`, `runtimehandlerhooks`, `statsserver`, `nri`

---

### Builder Interface

Builder pattern for constructing `Sandbox` instances. Provides setter
methods for all sandbox properties and validates configuration before
producing the sandbox.

```go
type Builder interface {
    SetConfig(*types.PodSandboxConfig) error
    GenerateNameAndID() error
    Config() *types.PodSandboxConfig
    ID() string
    Name() string
    InitInfraContainer(*libconfig.Config, *storage.ContainerInfo, *idtools.IDMappings) error
    Spec() *generate.Generator
    ResolvPath() string
    SetDNSConfig(*types.DNSConfig)
    SetCRISandbox(string, map[string]string, map[string]string, *types.PodSandboxMetadata) error
    GetSandbox() (*Sandbox, error)
    SetNamespace(string)
    SetName(string)
    SetKubeName(string)
    SetLogDir(string)
    SetContainers(memorystore.Storer[*oci.Container])
    SetProcessLabel(string)
    SetMountLabel(string)
    SetShmPath(string)
    SetCgroupParent(string)
    SetPrivileged(bool)
    SetRuntimeHandler(string)
    SetResolvPath(string)
    SetHostname(string)
    SetPortMappings([]*hostport.PortMapping)
    SetHostNetwork(bool)
    SetUsernsMode(string)
    SetPodLinuxOverhead(*types.LinuxContainerResources)
    SetPodLinuxResources(*types.LinuxContainerResources)
    SetHostnamePath(string)
    SetNamespaceOptions(*types.NamespaceOption)
    SetSeccompProfilePath(string)
    SetID(string)
    SetCreatedAt(time.Time)
    Validate() error
}
```

**Package:** `internal/lib/sandbox`

---

### Sandbox Constants

```go
const DevShmPath    = "/dev/shm"
const DefaultShmSize = 64 * 1024 * 1024  // 64 MiB

var ErrIDEmpty = errors.New("PodSandboxId should not be empty")
```

**Package:** `internal/lib/sandbox`

---

## Storage Types

### ContainerInfo

Wraps essential information about a container from the storage layer.

```go
type ContainerInfo struct {
    ID           string
    Dir          string       // nonvolatile per-container directory
    RunDir       string       // volatile per-container directory
    Config       *v1.Image    // image configuration blob
    ProcessLabel string       // SELinux process label
    MountLabel   string       // SELinux mount label
}
```

**Package:** `internal/storage`

**Produced by:** `RuntimeServer.CreatePodSandbox()`,
`RuntimeServer.CreateContainer()`

**Consumed by:** `server` (during `CreateContainer` and `RunPodSandbox`
flows)

---

### RuntimeContainerMetadata

Metadata stored alongside each container in containers/storage.

```go
type RuntimeContainerMetadata struct {
    PodName       string
    PodID         string
    ImageName     string
    ImageID       string
    ContainerName string
    MetadataName  string
    UID           string
    Namespace     string
    MountLabel    string
    CreatedAt     int64
    Attempt       uint32
    Pod           bool    // true if this is a pod sandbox
    Privileged    bool
}
```

**Package:** `internal/storage`

---

### ImageResult

Result of an image query from the storage layer.

```go
type ImageResult struct {
    ID                  StorageImageID
    SomeNameOfThisImage *RegistryImageReference
    RepoTags            []string
    RepoDigests         []string
    Size                *uint64
    Digest              digest.Digest
    User                string
    PreviousName        string
    Labels              map[string]string
    OCIConfig           *specs.Image
    Annotations         map[string]string
    Pinned              bool
    MountPoint          string
}
```

**Package:** `internal/storage`

**Consumed by:** `server` (`ConvertImage`, `CreateContainer` for image
config), `factory/container` (spec annotations)

---

### ImageCopyOptions

Options for image pull operations.

```go
type ImageCopyOptions struct {
    SystemContext     *types.SystemContext
    AuthFile          string
    CgroupPull        CgroupPullConfiguration
    StoreOptions      storage.ImageCopyOptions
    SourceCtx         *types.SystemContext
    DestinationCtx    *types.SystemContext
    OciDecryptConfig  *encconfig.DecryptConfig
    SignaturePolicyPath string
}
```

**Package:** `internal/storage`

---

### StorageImageID

Wraps a container image ID as stored in containers/storage.

```go
type StorageImageID struct {
    // unexported field wrapping digest
}
```

**Package:** `internal/storage/references`

Key methods:

```go
func ParseStorageImageIDFromOutOfProcessData(input string) (StorageImageID, error)
func (id StorageImageID) IDStringForOutOfProcessConsumptionOnly() string
```

---

### RegistryImageReference

A typed wrapper for a named image reference on a registry.

```go
type RegistryImageReference struct {
    privateNamed reference.Named  // unexported
}
```

**Package:** `internal/storage/references`

Key methods:

```go
func RegistryImageReferenceFromRaw(rawNamed reference.Named) RegistryImageReference
func ParseRegistryImageReferenceFromOutOfProcessData(input string) (RegistryImageReference, error)
func (ref RegistryImageReference) StringForOutOfProcessConsumptionOnly() string
func (ref RegistryImageReference) Registry() string
func (ref RegistryImageReference) Raw() reference.Named
```

---

### StorageTransport

Interface for resolving image references against the storage transport.

```go
type StorageTransport interface {
    ResolveReference(ref types.ImageReference) (types.ImageReference, *storage.Image, error)
}
```

**Package:** `internal/storage`

---

## Configuration Types

### Seccomp Config

Global seccomp configuration.

```go
type Config struct {
    enabled      bool
    profile      *seccomp.Seccomp
    notifierPath string
}
```

**Package:** `internal/config/seccomp`

---

### Seccomp Notifier and Notification

The `Notifier` listens for seccomp events from the kernel via a Unix
socket. `Notification` represents a single seccomp event.

```go
type Notifier struct {
    // fields: listener, container, stopContainers, timer, usedSyscalls, etc.
}

type Notification struct {
    ctx         context.Context
    containerID string
    syscall     string
}
```

**Package:** `internal/config/seccomp`

---

### CgroupManager Interface

Abstraction over cgroup management operations.

```go
type CgroupManager interface {
    Name() string
    IsSystemd() bool
    ContainerCgroupPath(string, string) string
    ContainerCgroupAbsolutePath(string, string) (string, error)
    ContainerCgroupManager(sbParent, containerID string) (cgroups.Manager, error)
    RemoveContainerCgManager(containerID string)
    ContainerCgroupStats(sbParent, containerID string) (*stats.CgroupStats, error)
    SandboxCgroupPath(string, string, int64) (string, string, error)
    SandboxCgroupManager(sbParent, sbID string) (cgroups.Manager, error)
    RemoveSandboxCgManager(sbID string)
    MoveConmonToCgroup(cid, cgroupParent, conmonCgroup string, pid int, resources *rspec.LinuxResources) (string, error)
    CreateSandboxCgroup(sbParent, containerID string) error
    RemoveSandboxCgroup(sbParent, containerID string) error
    SandboxCgroupStats(sbParent, sbID string) (*stats.CgroupStats, error)
    ExecCgroupManager(cgroupPath string) (cgroups.Manager, error)
    PodAndContainerCgroupManagers(sbParent, containerID string) (cgroups.Manager, []cgroups.Manager, error)
}
```

**Package:** `internal/config/cgmgr`

---

### CgroupManager Implementations

| Type                | Description                                  |
| ------------------- | -------------------------------------------- |
| `SystemdManager`    | Uses systemd to manage cgroups via D-Bus     |
| `CgroupfsManager`   | Directly manages cgroup filesystem           |
| `NullCgroupManager` | No-op implementation for non-Linux platforms |

**Package:** `internal/config/cgmgr`

---

### CNIManager

Manages the Container Network Interface plugin lifecycle.

```go
type CNIManager struct {
    // fields: plugin, defaultNetwork, networkDir, pluginDirs, watchers, etc.
}

type PodNetworkLister func() ([]*ocicni.PodNetwork, error)
```

**Package:** `internal/config/cnimgr`

---

### ConmonManager

Manages the conmon binary and probes its capabilities.

```go
type ConmonManager struct {
    // fields: conmonPath, features
}
```

**Package:** `internal/config/conmonmgr`

---

### Device Config

Manages additional device passthrough configuration.

```go
type Config struct {
    devices []Device
}

type Device struct {
    Device rspec.LinuxDevice
    Cgroup rspec.LinuxDeviceCgroup
}
```

**Package:** `internal/config/device`

**Constant:** `DeviceAnnotationDelim` -- delimiter used in the
`io.kubernetes.cri-o.Devices` annotation.

---

### NamespaceManager

Manages Linux namespace creation and lifecycle via the `pinns` utility.

```go
type NamespaceManager struct {
    namespacesDir string
    pinnsPath     string
}
```

**Package:** `internal/config/nsmgr`

---

### Namespace Interface

```go
type Namespace interface {
    Path() string
    Type() NSType
    Remove() error
}
```

**Package:** `internal/config/nsmgr`

---

### PodNamespacesConfig

Configuration for creating pod-level namespaces.

```go
type PodNamespacesConfig struct {
    Namespaces []*PodNamespaceConfig
}

type PodNamespaceConfig struct {
    Type NSType
    Host bool
    Path string
}
```

**Package:** `internal/config/nsmgr`

---

### RDT Config

Intel Resource Director Technology configuration.

```go
type Config struct {
    // fields: enabled, supported, configPath, etc.
}
```

**Package:** `internal/config/rdt`

**Constants:** `DefaultRdtConfigFile`, `ResctrlPrefix`

---

### AppArmor Config

Global AppArmor configuration.

```go
type Config struct {
    // fields: enabled, defaultProfile
}
```

**Package:** `internal/config/apparmor`

**Constant:** `DefaultProfile = "crio-default"`

---

### BlockIO Config

Block I/O tuning configuration.

```go
type Config struct {
    // fields: enabled, path, reload
}
```

**Package:** `internal/config/blockio`

---

### Capabilities

Default Linux capabilities for containers.

```go
type Capabilities []string
```

**Package:** `internal/config/capabilities`

---

### NRI Config

Node Resource Interface configuration.

```go
type Config struct {
    // fields: enabled, socketPath, pluginPath, etc.
}

type DefaultValidatorConfig struct {
    // fields mirror NRI validator config
}
```

**Package:** `internal/config/nri`

---

### Ulimits Config

```go
type Config struct {
    ulimits []Ulimit
}

type Ulimit struct {
    Name string
    Hard uint64
    Soft uint64
}
```

**Package:** `internal/config/ulimits`

---

## Runtime Types

### Runtime

The runtime manager that dispatches operations to the appropriate
`RuntimeImpl` based on the container's runtime handler configuration.

```go
type Runtime struct {
    config              *config.Config
    runtimeImplMap      map[string]RuntimeImpl
    runtimeImplMapMutex sync.RWMutex
}
```

**Package:** `internal/oci`

**Created by:** [`oci.New()`](api-reference.md#new-2)

**Used by:** `lib.ContainerServer` (via `Runtime()` accessor)

---

### RuntimeImpl Interface

The pluggable interface for OCI runtime implementations.

```go
type RuntimeImpl interface {
    CreateContainer(context.Context, *Container, string, bool) error
    StartContainer(context.Context, *Container) error
    ExecContainer(context.Context, *Container, []string, io.Reader, io.WriteCloser, io.WriteCloser,
        bool, <-chan remotecommand.TerminalSize) error
    ExecSyncContainer(context.Context, *Container, []string, int64) (*types.ExecSyncResponse, error)
    UpdateContainer(context.Context, *Container, *rspec.LinuxResources) error
    StopContainer(context.Context, *Container, int64) error
    DeleteContainer(context.Context, *Container) error
    UpdateContainerStatus(context.Context, *Container) error
    PauseContainer(context.Context, *Container) error
    UnpauseContainer(context.Context, *Container) error
    CgroupStats(context.Context, *Container, string) (*stats.CgroupStats, error)
    DiskStats(context.Context, *Container, string) (*stats.DiskStats, error)
    AttachContainer(context.Context, *Container, io.Reader, io.WriteCloser, io.WriteCloser,
        bool, <-chan remotecommand.TerminalSize) error
    PortForwardContainer(context.Context, *Container, string, int32, io.ReadWriteCloser) error
    ReopenContainerLog(context.Context, *Container) error
    CheckpointContainer(context.Context, *Container, *rspec.Spec, bool) error
    RestoreContainer(context.Context, *Container, string, string) error
    IsContainerAlive(*Container) bool
    ProbeMonitor(context.Context, *Container) error
    ServeExecContainer(context.Context, *Container, []string, bool, bool, bool, bool) (string, error)
    ServeAttachContainer(context.Context, *Container, bool, bool, bool) (string, error)
}
```

**Package:** `internal/oci`

---

### Runtime Implementations

| Type         | Description                           | Monitor            |
| ------------ | ------------------------------------- | ------------------ |
| `runtimeOCI` | Traditional OCI runtimes (runc, crun) | conmon             |
| `runtimeVM`  | VM-based runtimes (Kata Containers)   | ttrpc task service |
| `runtimePod` | Pod-level management                  | conmon-rs          |

All three are unexported types that implement the `RuntimeImpl` interface.
The `Runtime` struct selects the appropriate implementation based on the
runtime handler configuration.

**Package:** `internal/oci`

---

## Factory Types

### Container Interface

The container factory interface for building OCI specs from CRI
configuration.

```go
type Container interface {
    SetConfig(*types.ContainerConfig, *types.PodSandboxConfig) error
    SetNameAndID(string) error
    Config() *types.ContainerConfig
    SandboxConfig() *types.PodSandboxConfig
    ID() string
    Name() string
    SetPrivileged() error
    Privileged() bool
    LogPath(string) (string, error)
    DisableFips() bool
    UserRequestedImage() (string, error)
    ReadOnly(bool) bool
    SelinuxLabel(string) ([]string, error)
    SetRestore(bool)
    Restore() bool
    Spec() *generate.Generator
    SpecAddMount(rspec.Mount)
    SpecAddAnnotations(ctx context.Context, sb SandboxIFace, containerVolume []oci.ContainerVolume, mountPoint, configStopSignal string, imageResult *storage.ImageResult, isSystemd bool, seccompRef, platformRuntimePath string) error
    SpecAddDevices([]device.Device, []device.Device, bool, bool) error
    SpecInjectCDIDevices() error
    AddUnifiedResourcesFromAnnotations(map[string]string) error
    SpecSetProcessArgs(*v1.Image) error
    SpecAddNamespaces(SandboxIFace, *oci.Container, *config.Config) error
    SpecSetupCapabilities(*types.Capability, capabilities.Capabilities, bool) error
    SpecSetPrivileges(context.Context, *types.LinuxContainerSecurityContext, *config.Config) error
    SpecSetLinuxContainerResources(*types.LinuxContainerResources, int64) error
    PidNamespace() nsmgr.Namespace
    WillRunSystemd() bool
}
```

**Package:** `internal/factory/container`

**Created by:** [`container.New()`](api-reference.md#new-4)

---

### SandboxIFace

Minimal sandbox interface consumed by the container factory.

```go
type SandboxIFace interface {
    // methods for namespace paths, labels, annotations, etc.
}
```

**Package:** `internal/factory/container`

---

## Hook Types

### RuntimeHandlerHooks Interface

Lifecycle hooks that run at specific points during container creation and
teardown. Used for runtime-specific behavior such as CPU pinning for
high-performance workloads.

```go
type RuntimeHandlerHooks interface {
    PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, c *oci.Container) error
    PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
    PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
    PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}
```

**Package:** `internal/runtimehandlerhooks`

| Hook        | When                                             |
| ----------- | ------------------------------------------------ |
| `PreCreate` | After OCI spec generation, before runtime create |
| `PreStart`  | After container is created, before runtime start |
| `PreStop`   | Before stopping the container                    |
| `PostStop`  | After the container has been stopped             |

---

### HighPerformanceHook Interface

Extends `RuntimeHandlerHooks` for high-performance workloads (CPU pinning,
IRQ balance, etc.).

```go
type HighPerformanceHook interface {
    RuntimeHandlerHooks
}
```

**Package:** `internal/runtimehandlerhooks`

---

### HooksRetriever

Resolves the appropriate hooks implementation for a given runtime handler.

```go
type HooksRetriever struct {
    config               *libconfig.Config
    highPerformanceHooks RuntimeHandlerHooks
}
```

**Package:** `internal/runtimehandlerhooks`

---

## Host Port Types

### HostPortManager Interface

Manages host-to-container port mappings via iptables or nftables.

```go
type HostPortManager interface {
    Add(id, name, podIP string, hostportMappings []*PortMapping) error
    Remove(id string, hostportMappings []*PortMapping) error
}
```

**Package:** `internal/hostport`

---

### PortMapping

Represents a network port mapping between host and container.

```go
type PortMapping struct {
    HostPort      int32
    ContainerPort int32
    Protocol      v1.Protocol
    HostIP        string
}
```

**Package:** `internal/hostport`

---

## Stats Types

### StatsServer

Collects, caches, and serves container and sandbox resource statistics.

```go
type StatsServer struct {
    // fields: container/sandbox stats caches, collection period, shutdown channel
}
```

**Package:** `internal/lib/statsserver`

---

### SandboxMetrics

Extended metrics for a pod sandbox (beyond basic stats).

```go
type SandboxMetrics struct {
    // fields: pod-level and per-container metrics
}
```

**Package:** `internal/lib/statsserver`

---

### DiskStats and FilesystemStats

Disk usage statistics for a container.

```go
type DiskStats struct {
    Filesystem FilesystemStats
}

type FilesystemStats struct {
    UsageBytes  uint64
    LimitBytes  uint64
    InodesFree  uint64
    InodesTotal uint64
}
```

**Package:** `internal/lib/stats`

---

### CgroupStats

Cgroup resource usage statistics. Wraps libcontainer's `Stats` with
additional process-level data.

```go
type CgroupStats struct {
    cgroups.Stats
    ProcessStats ProcessStats
    SystemNano   int64
}

type ProcessStats struct {
    Pids            []int
    FileDescriptors uint64
    Sockets         uint64
    Threads         uint64
    ThreadsMax      uint64
    UlimitsSoft     uint64
}
```

**Package:** `internal/lib/stats` (Linux only; empty struct on other
platforms)

---

## Metrics Types

### Metrics

Prometheus metrics for CRI-O operations.

```go
type Metrics struct {
    // fields: histogram/counter/gauge metrics for operations, image pulls,
    // container lifecycle, OOM events, etc.
}
```

**Package:** `server/metrics`

---

### Collector and Collectors

```go
type Collector string
type Collectors []Collector
```

**Package:** `server/metrics/collectors`

Functions:

```go
func FromSlice(in []string) Collectors
func All() Collectors
```

---

## Utility Types

### Error Sentinels

Pre-defined error values that map to gRPC status codes.

```go
var ErrUnknown           = errors.New("unknown")
var ErrInvalidArgument   = errors.New("invalid argument")
var ErrNotFound          = errors.New("not found")
var ErrAlreadyExists     = errors.New("already exists")
var ErrFailedPrecondition = errors.New("failed precondition")
var ErrUnavailable       = errors.New("unavailable")
var ErrNotImplemented    = errors.New("not implemented")
```

**Package:** `utils/errdefs`

---

### CommandRunner Interface

```go
type CommandRunner interface {
    Command(string, ...string) *exec.Cmd
    CommandContext(context.Context, string, ...string) *exec.Cmd
    CombinedOutput(string, ...string) ([]byte, error)
}
```

**Package:** `utils/cmdrunner`

---

### DetachError

Returned when a container exec or attach session is detached by the user
via the detach key sequence.

```go
type DetachError struct{}

func (DetachError) Error() string
```

**Package:** `utils`

---

### VersionInfo

Used to construct the User-Agent string for image registry requests.

```go
type VersionInfo struct {
    Name    string
    Version string
}
```

**Package:** `server/useragent`

---

## Checkpoint Types

### ContainerCheckpointOptions

Options for container checkpoint and restore operations.

```go
type ContainerCheckpointOptions struct {
    Keep        bool    // Do not delete checkpoint artifacts
    KeepRunning bool    // Keep the container running after checkpoint
    TargetFile  string  // Read/write checkpoint image from/to this file
}
```

**Package:** `internal/lib`

---

### ExternalBindMount

Tracks bind mounts that need to be restored when restoring a checkpointed
container.

```go
type ExternalBindMount struct {
    Source      string `json:"source"`
    Destination string `json:"destination"`
    FileType    string `json:"file_type"`
    Permissions uint32 `json:"permissions"`
}
```

**Package:** `internal/lib`

---

## Constants

### Library Constants

```go
const ContainerManagerCRIO = "cri-o"
```

**Package:** `internal/lib/constants`

```go
const PodCgroupName = "pod"
```

**Package:** `utils`

---

### Namespace Type Constants

```go
const (
    NETNS  NSType = "net"
    IPCNS  NSType = "ipc"
    UTSNS  NSType = "uts"
    USERNS NSType = "user"
    PIDNS  NSType = "pid"
    ManagedNamespacesNum = 5
)
```

**Package:** `internal/config/nsmgr`

---

### Cgroup Constants

```go
const CrioPrefix              = "crio"
const CgroupMemoryPathV1      = "/sys/fs/cgroup/memory"
const CgroupMemoryPathV2      = "/sys/fs/cgroup"
const DefaultCgroupManager    = "systemd"
```

**Package:** `internal/config/cgmgr`
