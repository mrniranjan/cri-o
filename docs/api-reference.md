# CRI-O Public API Reference

<!-- toc -->

- [Package <code>server</code>](#package-server)
  - [Server Lifecycle](#server-lifecycle)
    - [<code>New</code>](#new)
    - [<code>StopStreamServer</code>](#stopstreamserver)
    - [<code>StopMonitors</code>](#stopmonitors)
    - [<code>StartExitMonitor</code>](#startexitmonitor)
    - [<code>Listen</code>](#listen)
    - [<code>StopPodSandbox</code>](#stoppodsandbox)
    - [<code>PodSandboxStatus</code>](#podsandboxstatus)
    - [<code>PodSandboxStats</code>](#podsandboxstats)
    - [<code>UpdatePodSandboxResources</code>](#updatepodsandboxresources)
    - [<code>StartContainer</code>](#startcontainer)
    - [<code>RemoveContainer</code>](#removecontainer)
    - [<code>ContainerStatus</code>](#containerstatus)
    - [<code>ListContainerStats</code>](#listcontainerstats)
    - [<code>ReopenContainerLog</code>](#reopencontainerlog)
    - [<code>GetContainerEvents</code>](#getcontainerevents)
    - [<code>ExecSync</code>](#execsync)
    - [<code>PortForward</code>](#portforward)
    - [<code>Status</code>](#status)
    - [<code>UpdateRuntimeConfig</code>](#updateruntimeconfig)
    - [<code>ListImages</code>](#listimages)
    - [<code>RemoveImage</code>](#removeimage)
    - [<code>ConvertImage</code>](#convertimage)
  - [Helpers and Utilities](#helpers-and-utilities)
    - [<code>ReserveSandboxContainerIDAndName</code>](#reservesandboxcontaineridandname)
    - [<code>CRImportCheckpoint</code>](#crimportcheckpoint)
    - [<code>InitLabel</code>](#initlabel)
    - [Accessor Methods](#accessor-methods)
    - [<code>GetContainer</code> / <code>GetInfraContainer</code>](#getcontainer--getinfracontainer)
    - [<code>RemoveContainer</code> / <code>RemoveInfraContainer</code>](#removecontainer--removeinfracontainer)
    - [<code>GetContainerFromShortID</code>](#getcontainerfromshortid)
  - [Sandbox Management](#sandbox-management)
    - [<code>AddSandbox</code>](#addsandbox)
    - [<code>GetSandboxContainer</code>](#getsandboxcontainer)
    - [<code>RemoveSandbox</code>](#removesandbox)
  - [Name Reservation](#name-reservation)
    - [<code>ReserveContainerName</code> / <code>ReleaseContainerName</code>](#reservecontainername--releasecontainername)
    - [<code>ContainerIDForName</code> / <code>PodIDForName</code>](#containeridforname--podidforname)
    - [<code>LoadSandbox</code> / <code>LoadContainer</code>](#loadsandbox--loadcontainer)
    - [<code>ShutdownWasUnclean</code>](#shutdownwasunclean)
    - [<code>ContainerRestore</code>](#containerrestore)
  - [Sandbox Accessors](#sandbox-accessors)
- [Package <code>internal/lib/statsserver</code>](#package-internallibstatsserver)
  - [<code>New</code>](#new-1)
  - [Container Stats](#container-stats)
  - [Sandbox Stats](#sandbox-stats)
  - [Metrics](#metrics)
  - [Lifecycle](#lifecycle)
  - [Runtime Configuration Queries](#runtime-configuration-queries)
  - [Runtime Stats and I/O](#runtime-stats-and-io)
  - [Container](#container)
    - [<code>NewContainer</code>](#newcontainer)
    - [<code>ReadConmonPidFile</code>](#readconmonpidfile)
- [Package <code>internal/storage</code>](#package-internalstorage)
  - [ImageServer](#imageserver)
    - [<code>GetImageService</code>](#getimageservice)
  - [Package-Level Functions](#package-level-functions)
- [Package <code>internal/runtimehandlerhooks</code>](#package-internalruntimehandlerhooks)
  - [<code>RuntimeHandlerHooks</code> Interface](#runtimehandlerhooks-interface)
  - [<code>NewHooksRetriever</code>](#newhooksretriever)
  - [<code>Get</code>](#get)
  - [<code>RestoreIrqBalanceConfig</code>](#restoreirqbalanceconfig)
- [Package <code>internal/config</code> Subsystems](#package-internalconfig-subsystems)
  - [seccomp](#seccomp)
  - [cgmgr](#cgmgr)
  - [cnimgr](#cnimgr)
  - [conmonmgr](#conmonmgr)
  - [device](#device)
  - [nsmgr](#nsmgr)
  - [node](#node)
  - [rdt](#rdt)
  - [apparmor](#apparmor)
  - [blockio](#blockio)
  - [capabilities](#capabilities)
  - [nri](#nri)
  - [ulimits](#ulimits)
- [Package <code>utils/errdefs</code>](#package-utilserrdefs)
- [Package <code>utils/cmdrunner</code>](#package-utilscmdrunner)
<!-- /toc -->

## Package `server`

The `server` package implements the Kubernetes CRI gRPC services
(`RuntimeService` and `ImageService`). The `Server` struct is the central
object that receives all CRI requests from the kubelet.

See also: [Data Structures Reference](data-structures.md) |
[Container Creation Flow](container-creation-flow.md)

### Server Lifecycle

#### `New`

Creates and initializes the CRI-O gRPC server, including the streaming
server, storage, runtime, NRI, and exit monitors.

```go
func New(ctx context.Context, configIface libconfig.Iface) (*Server, error)
```

**Parameters:**

- `ctx` -- parent context for the server's lifetime.
- `configIface` -- validated CRI-O configuration interface.

**Returns:** a fully initialized `*Server` or an error if configuration
validation, storage initialization, or runtime setup fails.

**Errors:** returns errors from configuration validation, container storage
initialization, runtime creation, or streaming server setup.

**See also:**
[`lib.ContainerServer.New`](#containerserver),
[`oci.Runtime.New`](#runtime)

---

#### `Shutdown`

Gracefully shuts down the server, cleaning up storage, stopping monitors,
and closing the streaming server.

```go
func (s *Server) Shutdown(ctx context.Context) error
```

**Errors:** returns errors from the underlying storage shutdown.

---

#### `StopStreamServer`

Stops the streaming server used for Exec, Attach, and PortForward.

```go
func (s *Server) StopStreamServer() error
```

---

#### `StreamingServerCloseChan`

Returns the channel that is closed when the streaming server stops.

```go
func (s *Server) StreamingServerCloseChan() chan struct{}
```

---

#### `StopMonitors`

Stops all container exit monitors.

```go
func (s *Server) StopMonitors()
```

---

#### `MonitorsCloseChan`

Returns the channel that is closed when the exit monitor stops.

```go
func (s *Server) MonitorsCloseChan() chan struct{}
```

---

#### `StartExitMonitor`

Starts a goroutine that watches `ContainerExitsDir` via fsnotify and
updates container status when exit files appear.

```go
func (s *Server) StartExitMonitor(ctx context.Context)
```

**See also:** [Container Creation Flow -- Exit Monitoring](container-creation-flow.md#exit-monitoring)

---

#### `ArtifactStore`

Returns the OCI artifact store instance used for seccomp profile artifacts.

```go
func (s *Server) ArtifactStore() *ociartifact.Store
```

---

#### `Listen`

Opens a Unix socket listener at the given address for the CRI-O gRPC
server.

```go
func Listen(network, address string) (net.Listener, error)
```

**Parameters:**

- `network` -- must be `"unix"`.
- `address` -- socket path (e.g. `/var/run/crio/crio.sock`).

**Returns:** a `net.Listener` or an error if binding fails.

---

### CRI RuntimeService -- Sandbox Operations

#### `RunPodSandbox`

Creates and runs a pod-level sandbox. Sets up namespaces, networking (CNI),
cgroups, and the infra container.

```go
func (s *Server) RunPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (*types.RunPodSandboxResponse, error)
```

**Parameters:**

- `req.Config` -- pod sandbox configuration (metadata, DNS, port mappings,
  labels, annotations, Linux-specific settings).
- `req.RuntimeHandler` -- selects the OCI runtime handler to use.

**Returns:** `PodSandboxId` of the created sandbox.

**Errors:** returns errors from namespace creation, CNI setup, storage
allocation, or OCI runtime failures.

---

#### `StopPodSandbox`

Stops the sandbox. Force-terminates any running containers in the sandbox.
Tears down networking via CNI.

```go
func (s *Server) StopPodSandbox(ctx context.Context, req *types.StopPodSandboxRequest) (*types.StopPodSandboxResponse, error)
```

**Parameters:**

- `req.PodSandboxId` -- ID of the sandbox to stop.

**Errors:** returns an error if the sandbox is not found or if container
stop or CNI teardown fails.

---

#### `RemovePodSandbox`

Deletes the sandbox. Force-deletes any remaining containers.

```go
func (s *Server) RemovePodSandbox(ctx context.Context, req *types.RemovePodSandboxRequest) (*types.RemovePodSandboxResponse, error)
```

---

#### `PodSandboxStatus`

Returns the status of the specified sandbox.

```go
func (s *Server) PodSandboxStatus(ctx context.Context, req *types.PodSandboxStatusRequest) (*types.PodSandboxStatusResponse, error)
```

---

#### `ListPodSandbox`

Returns a list of sandboxes matching the optional filter criteria.

```go
func (s *Server) ListPodSandbox(ctx context.Context, req *types.ListPodSandboxRequest) (*types.ListPodSandboxResponse, error)
```

---

#### `PodSandboxStats`

Returns resource usage statistics for the specified sandbox.

```go
func (s *Server) PodSandboxStats(ctx context.Context, req *types.PodSandboxStatsRequest) (*types.PodSandboxStatsResponse, error)
```

---

#### `ListPodSandboxStats`

Returns stats for all sandboxes matching the optional filter.

```go
func (s *Server) ListPodSandboxStats(ctx context.Context, req *types.ListPodSandboxStatsRequest) (*types.ListPodSandboxStatsResponse, error)
```

---

#### `UpdatePodSandboxResources`

Updates the cgroup resources for a sandbox.

```go
func (s *Server) UpdatePodSandboxResources(ctx context.Context, req *types.UpdatePodSandboxResourcesRequest) (*types.UpdatePodSandboxResourcesResponse, error)
```

---

### CRI RuntimeService -- Container Operations

#### `CreateContainer`

Creates a new container in the specified pod sandbox. This is the most
complex CRI operation -- it resolves the image, builds an OCI spec, sets up
storage, configures security profiles, and invokes the OCI runtime.

```go
func (s *Server) CreateContainer(ctx context.Context, req *types.CreateContainerRequest) (*types.CreateContainerResponse, error)
```

**Parameters:**

- `req.PodSandboxId` -- ID of the parent sandbox.
- `req.Config` -- container configuration (image, command, mounts,
  environment, devices, security context, resources).
- `req.SandboxConfig` -- sandbox configuration for context.

**Returns:** `ContainerId` of the created container.

**Errors:** returns errors from image resolution, storage creation, OCI
spec generation, security profile setup, or runtime creation. On error,
partially-created resources are cleaned up.

**See also:** [Container Creation Flow](container-creation-flow.md#createcontainer-flow)

---

#### `StartContainer`

Starts a previously created container. Invokes runtime handler hooks
(`PreStart`) and the OCI runtime.

```go
func (s *Server) StartContainer(ctx context.Context, req *types.StartContainerRequest) (*types.StartContainerResponse, error)
```

**Parameters:**

- `req.ContainerId` -- ID of the container to start.

**Errors:** returns an error if the container is not in `Created` state or
if the runtime fails to start it.

---

#### `StopContainer`

Stops a running container with a grace period (timeout in seconds).

```go
func (s *Server) StopContainer(ctx context.Context, req *types.StopContainerRequest) (*types.StopContainerResponse, error)
```

**Parameters:**

- `req.ContainerId` -- ID of the container to stop.
- `req.Timeout` -- grace period in seconds before force-killing.

---

#### `RemoveContainer`

Removes the container. If the container is running, it is force-terminated
first.

```go
func (s *Server) RemoveContainer(ctx context.Context, req *types.RemoveContainerRequest) (*types.RemoveContainerResponse, error)
```

---

#### `ListContainers`

Lists all containers matching the optional filter criteria.

```go
func (s *Server) ListContainers(ctx context.Context, req *types.ListContainersRequest) (*types.ListContainersResponse, error)
```

---

#### `ContainerStatus`

Returns the status of the specified container.

```go
func (s *Server) ContainerStatus(ctx context.Context, req *types.ContainerStatusRequest) (*types.ContainerStatusResponse, error)
```

---

#### `ContainerStats`

Returns resource usage statistics for the specified container.

```go
func (s *Server) ContainerStats(ctx context.Context, req *types.ContainerStatsRequest) (*types.ContainerStatsResponse, error)
```

---

#### `ListContainerStats`

Returns stats for all running containers matching the optional filter.

```go
func (s *Server) ListContainerStats(ctx context.Context, req *types.ListContainerStatsRequest) (*types.ListContainerStatsResponse, error)
```

---

#### `UpdateContainerResources`

Updates the resource constraints (CPU, memory, etc.) of a container.

```go
func (s *Server) UpdateContainerResources(ctx context.Context, req *types.UpdateContainerResourcesRequest) (*types.UpdateContainerResourcesResponse, error)
```

---

#### `ReopenContainerLog`

Reopens the container's log file. Called by the kubelet during log rotation.

```go
func (s *Server) ReopenContainerLog(ctx context.Context, req *types.ReopenContainerLogRequest) (*types.ReopenContainerLogResponse, error)
```

---

#### `CheckpointContainer`

Checkpoints a running container using CRIU.

```go
func (s *Server) CheckpointContainer(ctx context.Context, req *types.CheckpointContainerRequest) (*types.CheckpointContainerResponse, error)
```

---

#### `GetContainerEvents`

Sends a stream of container lifecycle events to the connected client.

```go
func (s *Server) GetContainerEvents(_ *types.GetEventsRequest, ces types.RuntimeService_GetContainerEventsServer) error
```

---

### CRI RuntimeService -- Streaming Operations

#### `Exec`

Prepares a streaming endpoint to execute a command in a container. Returns
a URL for the kubelet to connect to.

```go
func (s *Server) Exec(ctx context.Context, req *types.ExecRequest) (*types.ExecResponse, error)
```

The actual execution happens via the `StreamService`:

```go
func (s *StreamService) Exec(ctx context.Context, containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error
```

---

#### `ExecSync`

Runs a command in a container synchronously and returns stdout, stderr, and
exit code.

```go
func (s *Server) ExecSync(ctx context.Context, req *types.ExecSyncRequest) (*types.ExecSyncResponse, error)
```

---

#### `Attach`

Prepares a streaming endpoint to attach to a running container's stdio.

```go
func (s *Server) Attach(ctx context.Context, req *types.AttachRequest) (*types.AttachResponse, error)
```

The actual attach happens via the `StreamService`:

```go
func (s *StreamService) Attach(ctx context.Context, containerID string, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error
```

---

#### `PortForward`

Prepares a streaming endpoint to forward ports from a pod sandbox.

```go
func (s *Server) PortForward(ctx context.Context, req *types.PortForwardRequest) (*types.PortForwardResponse, error)
```

---

### CRI RuntimeService -- Runtime Operations

#### `Version`

Returns the runtime name, version, and API version.

```go
func (s *Server) Version(context.Context, *types.VersionRequest) (*types.VersionResponse, error)
```

---

#### `Status`

Returns the runtime status, including conditions for
`RuntimeReady` and `NetworkReady`.

```go
func (s *Server) Status(ctx context.Context, req *types.StatusRequest) (*types.StatusResponse, error)
```

---

#### `RuntimeConfig`

Returns configuration information of the runtime, such as the pod CIDR.

```go
func (s *Server) RuntimeConfig(_ context.Context, req *types.RuntimeConfigRequest) (*types.RuntimeConfigResponse, error)
```

---

#### `UpdateRuntimeConfig`

Updates runtime configuration (e.g. pod CIDR from the cluster).

```go
func (s *Server) UpdateRuntimeConfig(ctx context.Context, req *types.UpdateRuntimeConfigRequest) (*types.UpdateRuntimeConfigResponse, error)
```

---

### CRI ImageService

#### `PullImage`

Pulls an image with authentication credentials.

```go
func (s *Server) PullImage(ctx context.Context, req *types.PullImageRequest) (*types.PullImageResponse, error)
```

**Parameters:**

- `req.Image` -- image spec with the image name/reference.
- `req.Auth` -- optional authentication configuration.
- `req.SandboxConfig` -- sandbox context for the pull.

**Returns:** `ImageRef` of the pulled image.

**Errors:** returns errors from image resolution, registry authentication,
or download failures. Signature validation errors are wrapped with
`ErrSignatureValidationFailed`.

---

#### `ListImages`

Lists existing images in the local store.

```go
func (s *Server) ListImages(ctx context.Context, req *types.ListImagesRequest) (*types.ListImagesResponse, error)
```

---

#### `ImageStatus`

Returns the status of the specified image.

```go
func (s *Server) ImageStatus(ctx context.Context, req *types.ImageStatusRequest) (*types.ImageStatusResponse, error)
```

---

#### `RemoveImage`

Removes the specified image from the local store.

```go
func (s *Server) RemoveImage(ctx context.Context, req *types.RemoveImageRequest) (*types.RemoveImageResponse, error)
```

---

#### `ImageFsInfo`

Returns filesystem information for the image store.

```go
func (s *Server) ImageFsInfo(context.Context, *types.ImageFsInfoRequest) (*types.ImageFsInfoResponse, error)
```

---

#### `ConvertImage`

Converts a `storage.ImageResult` into a CRI protobuf `Image` type.

```go
func ConvertImage(from *storage.ImageResult) *types.Image
```

---

### HTTP Inspect API

#### `GetExtendInterfaceMux`

Returns an HTTP mux with endpoints for inspecting CRI-O internals. These
endpoints are served alongside gRPC via cmux on the same Unix socket.

```go
func (s *Server) GetExtendInterfaceMux(enableProfile bool) *chi.Mux
```

**Endpoints:**

| Path                | Description                                 |
| ------------------- | ------------------------------------------- |
| `/config`           | Current CRI-O configuration                 |
| `/info`             | Runtime and server information              |
| `/containers/:id`   | Inspect a container by ID                   |
| `/pause/:id`        | Pause a container                           |
| `/unpause/:id`      | Unpause a container                         |
| `/debug/goroutines` | Goroutine dump                              |
| `/debug/heap`       | Heap profile (when `enableProfile` is true) |

---

### Helpers and Utilities

#### `ReserveSandboxContainerIDAndName`

Generates and reserves a unique ID and name for a sandbox container.

```go
func (s *Server) ReserveSandboxContainerIDAndName(config *types.PodSandboxConfig) (string, error)
```

---

#### `FilterDisallowedAnnotations`

Removes annotations that are not in the runtime handler's allowed list.

```go
func (s *Server) FilterDisallowedAnnotations(toFind, toFilter map[string]string, runtimeHandler string) error
```

---

#### `CRImportCheckpoint`

Imports a checkpoint archive for container restore.

```go
func (s *Server) CRImportCheckpoint(ctx context.Context, createConfig *types.ContainerConfig, sb *sandbox.Sandbox, sandboxUID string) (string, error)
```

---

#### `KVMLabel`

Returns SELinux labels for running KVM-isolated containers.

```go
func KVMLabel(cLabel string) (string, error)
```

---

#### `InitLabel`

Returns SELinux labels for running systemd-based containers.

```go
func InitLabel(cLabel string) (string, error)
```

---

## Package `internal/lib`

The `lib` package provides the core container and sandbox management layer
that the `server` package builds upon.

### ContainerServer

#### `New`

Creates a new `ContainerServer` with storage, runtime, and in-memory
indexes initialized.

```go
func New(ctx context.Context, configIface libconfig.Iface) (*ContainerServer, error)
```

**Parameters:**

- `ctx` -- parent context.
- `configIface` -- validated configuration.

**Returns:** a fully initialized `*ContainerServer`.

**Errors:** returns errors from storage initialization, image server
creation, or runtime setup.

---

#### Accessor Methods

```go
func (c *ContainerServer) Runtime() *oci.Runtime
func (c *ContainerServer) Store() cstorage.Store
func (c *ContainerServer) StorageImageServer() storage.ImageServer
func (c *ContainerServer) StorageRuntimeServer() storage.RuntimeServer
func (c *ContainerServer) CtrIDIndex() *truncindex.TruncIndex
func (c *ContainerServer) PodIDIndex() *truncindex.TruncIndex
func (c *ContainerServer) Config() *libconfig.Config
```

These return the respective subsystem instances used by the server.

---

### Container Management

#### `AddContainer` / `AddInfraContainer`

Adds a container to the in-memory state store.

```go
func (c *ContainerServer) AddContainer(ctx context.Context, ctr *oci.Container)
func (c *ContainerServer) AddInfraContainer(ctx context.Context, ctr *oci.Container)
```

---

#### `GetContainer` / `GetInfraContainer`

Returns a container from the state store by ID.

```go
func (c *ContainerServer) GetContainer(ctx context.Context, id string) *oci.Container
func (c *ContainerServer) GetInfraContainer(ctx context.Context, id string) *oci.Container
```

---

#### `HasContainer`

Checks if a container exists in the state.

```go
func (c *ContainerServer) HasContainer(id string) bool
```

---

#### `RemoveContainer` / `RemoveInfraContainer`

Removes a container from the in-memory state store.

```go
func (c *ContainerServer) RemoveContainer(ctx context.Context, ctr *oci.Container)
func (c *ContainerServer) RemoveInfraContainer(ctx context.Context, ctr *oci.Container)
```

---

#### `ListContainers`

Returns all containers matching the optional filter functions.

```go
func (c *ContainerServer) ListContainers(filters ...func(*oci.Container) bool) ([]*oci.Container, error)
```

---

#### `GetContainerFromShortID`

Looks up a container by a prefix of its ID.

```go
func (c *ContainerServer) GetContainerFromShortID(ctx context.Context, cid string) (*oci.Container, error)
```

---

#### `UpdateContainerLinuxResources`

Updates the Linux resource constraints for a container.

```go
func (c *ContainerServer) UpdateContainerLinuxResources(ctr *oci.Container, resources *rspec.LinuxResources)
```

---

### Sandbox Management

#### `AddSandbox`

Adds a sandbox to the state store.

```go
func (c *ContainerServer) AddSandbox(ctx context.Context, sb *sandbox.Sandbox) error
```

---

#### `GetSandbox`

Returns a sandbox by ID.

```go
func (c *ContainerServer) GetSandbox(id string) *sandbox.Sandbox
```

---

#### `GetSandboxContainer`

Returns a sandbox's infra container.

```go
func (c *ContainerServer) GetSandboxContainer(id string) *oci.Container
```

---

#### `HasSandbox`

Checks if a sandbox exists in the state.

```go
func (c *ContainerServer) HasSandbox(id string) bool
```

---

#### `RemoveSandbox`

Removes a sandbox from the state store.

```go
func (c *ContainerServer) RemoveSandbox(ctx context.Context, id string) error
```

---

#### `ListSandboxes`

Lists all sandboxes in the state store.

```go
func (c *ContainerServer) ListSandboxes() []*sandbox.Sandbox
```

---

### Name Reservation

#### `ReserveContainerName` / `ReleaseContainerName`

Reserves or releases a container name to prevent duplicates.

```go
func (c *ContainerServer) ReserveContainerName(id, name string) (string, error)
func (c *ContainerServer) ReleaseContainerName(ctx context.Context, name string)
```

---

#### `ReservePodName` / `ReleasePodName`

Reserves or releases a pod name.

```go
func (c *ContainerServer) ReservePodName(id, name string) (string, error)
func (c *ContainerServer) ReleasePodName(name string)
```

---

#### `ContainerIDForName` / `PodIDForName`

Looks up a container or pod ID by name.

```go
func (c *ContainerServer) ContainerIDForName(name string) (string, error)
func (c *ContainerServer) PodIDForName(name string) (string, error)
```

---

### Storage and State

#### `ContainerStateToDisk`

Persists a container's state to a JSON file on disk.

```go
func (c *ContainerServer) ContainerStateToDisk(ctx context.Context, ctr *oci.Container) error
```

---

#### `LoadSandbox` / `LoadContainer`

Restores a sandbox or container from disk into the in-memory state store.
Used during server startup to recover state after restart.

```go
func (c *ContainerServer) LoadSandbox(ctx context.Context, id string) (*sandbox.Sandbox, error)
func (c *ContainerServer) LoadContainer(ctx context.Context, id string) error
```

---

#### `Shutdown`

Shuts down the container storage cleanly.

```go
func (c *ContainerServer) Shutdown() error
```

---

#### `ShutdownWasUnclean`

Returns whether the previous server shutdown was unclean (crash/kill).

```go
func ShutdownWasUnclean(config *libconfig.Config) bool
```

---

### Checkpoint and Restore

#### `ContainerCheckpoint`

Checkpoints a running container using CRIU.

```go
func (c *ContainerServer) ContainerCheckpoint(ctx context.Context, config *metadata.ContainerConfig, opts *ContainerCheckpointOptions) (string, error)
```

---

#### `ContainerRestore`

Restores a checkpointed container.

```go
func (c *ContainerServer) ContainerRestore(ctx context.Context, config *metadata.ContainerConfig, opts *ContainerCheckpointOptions) (string, error)
```

---

## Package `internal/lib/sandbox`

### Sandbox Builder

#### `NewBuilder`

Creates a new, empty sandbox builder for constructing `Sandbox` instances.

```go
func NewBuilder() Builder
```

The `Builder` interface exposes setter methods for all sandbox properties:

```go
type Builder interface {
    SetConfig(*types.PodSandboxConfig) error
    GenerateNameAndID() error
    Config() *types.PodSandboxConfig
    ID() string
    Name() string
    InitInfraContainer(*libconfig.Config, *storage.ContainerInfo, *idtools.IDMappings) error
    Spec() *generate.Generator
    SetCRISandbox(string, map[string]string, map[string]string, *types.PodSandboxMetadata) error
    GetSandbox() (*Sandbox, error)
    Validate() error
    // ... additional setters for all sandbox properties
}
```

Call `GetSandbox()` after configuring all properties to obtain the
immutable `Sandbox` object.

---

### Sandbox Accessors

The `Sandbox` struct exposes read-only accessors for all pod properties:

| Method             | Returns                     | Description                |
| ------------------ | --------------------------- | -------------------------- |
| `ID()`             | `string`                    | Pod sandbox ID             |
| `Name()`           | `string`                    | Generated name             |
| `KubeName()`       | `string`                    | Kubernetes pod name        |
| `Namespace()`      | `string`                    | Kubernetes namespace       |
| `Labels()`         | `fields.Set`                | Pod labels                 |
| `Annotations()`    | `map[string]string`         | Pod annotations            |
| `Metadata()`       | `*types.PodSandboxMetadata` | CRI metadata               |
| `CgroupParent()`   | `string`                    | Cgroup parent path         |
| `RuntimeHandler()` | `string`                    | Runtime handler name       |
| `Privileged()`     | `bool`                      | Privileged flag            |
| `HostNetwork()`    | `bool`                      | Host network flag          |
| `PortMappings()`   | `[]*hostport.PortMapping`   | Port mappings              |
| `IPs()`            | `[]string`                  | Pod IP addresses           |
| `LogDir()`         | `string`                    | Log directory              |
| `ShmPath()`        | `string`                    | Shared memory path         |
| `ResolvPath()`     | `string`                    | DNS resolv.conf path       |
| `Hostname()`       | `string`                    | Pod hostname               |
| `ProcessLabel()`   | `string`                    | SELinux process label      |
| `MountLabel()`     | `string`                    | SELinux mount label        |
| `Containers()`     | `Storer[*oci.Container]`    | Container store            |
| `InfraContainer()` | `*oci.Container`            | Infra container            |
| `Stopped()`        | `bool`                      | Whether sandbox is stopped |
| `Ready()`          | `bool`                      | Whether sandbox is ready   |

Container management on the sandbox:

```go
func (s *Sandbox) AddContainer(ctx context.Context, c *oci.Container)
func (s *Sandbox) GetContainer(ctx context.Context, name string) *oci.Container
func (s *Sandbox) RemoveContainer(ctx context.Context, c *oci.Container)
func (s *Sandbox) SetInfraContainer(infraCtr *oci.Container) error
func (s *Sandbox) RemoveInfraContainer()
```

State management:

```go
func (s *Sandbox) SetStopped(ctx context.Context, createFile bool)
func (s *Sandbox) SetCreated()
func (s *Sandbox) SetNetworkStopped(ctx context.Context, createFile bool) error
```

---

### Sandbox Namespace Methods

```go
func (s *Sandbox) NamespacePaths() []*namespace.ManagedNamespace
func (s *Sandbox) NetNsPath() string
func (s *Sandbox) IpcNsPath() string
func (s *Sandbox) UtsNsPath() string
func (s *Sandbox) UserNsPath() string
func (s *Sandbox) PidNsPath() string
func (s *Sandbox) AddManagedNamespaces(namespaces []nsmgr.Namespace)
func (s *Sandbox) RemoveManagedNamespaces() error
func (s *Sandbox) NetNsJoin(nspath string) error
func (s *Sandbox) IpcNsJoin(nspath string) error
func (s *Sandbox) UtsNsJoin(nspath string) error
func (s *Sandbox) UserNsJoin(nspath string) error
```

---

## Package `internal/lib/statsserver`

The `StatsServer` collects and caches container and sandbox resource usage
statistics.

#### `New`

```go
func New(ctx context.Context, cs parentServerIface) *StatsServer
```

#### Container Stats

```go
func (ss *StatsServer) StatsForContainer(c *oci.Container, sb *sandbox.Sandbox) *types.ContainerStats
func (ss *StatsServer) StatsForContainers(ctrs []*oci.Container) []*types.ContainerStats
func (ss *StatsServer) RemoveStatsForContainer(c *oci.Container)
```

#### Sandbox Stats

```go
func (ss *StatsServer) StatsForSandbox(sb *sandbox.Sandbox) *types.PodSandboxStats
func (ss *StatsServer) StatsForSandboxes(sboxes []*sandbox.Sandbox) []*types.PodSandboxStats
func (ss *StatsServer) RemoveStatsForSandbox(sb *sandbox.Sandbox)
```

#### Metrics

```go
func (ss *StatsServer) MetricsForPodSandbox(sb *sandbox.Sandbox) *SandboxMetrics
func (ss *StatsServer) MetricsForPodSandboxList(sboxes []*sandbox.Sandbox) []*SandboxMetrics
func (ss *StatsServer) PopulateMetricDescriptors(includedKeys []string) map[string][]*types.MetricDescriptor
```

#### Lifecycle

```go
func (ss *StatsServer) Shutdown()
```

---

## Package `internal/oci`

The `oci` package abstracts OCI container runtimes behind the `RuntimeImpl`
interface. CRI-O supports three runtime implementations:

- **runtimeOCI** -- traditional runtimes (runc, crun) via conmon
- **runtimeVM** -- VM-based runtimes (Kata Containers) via ttrpc
- **runtimePod** -- pod-level management via conmon-rs

### Runtime

#### `New`

Creates a new `Runtime` manager from the CRI-O configuration.

```go
func New(c *config.Config) (*Runtime, error)
```

---

### Runtime Configuration Queries

```go
func (r *Runtime) Runtimes() config.Runtimes
func (r *Runtime) ValidateRuntimeHandler(handler string) (*config.RuntimeHandler, error)
func (r *Runtime) PrivilegedWithoutHostDevices(handler string) (bool, error)
func (r *Runtime) PlatformRuntimePath(handler, platform string) (string, error)
func (r *Runtime) AllowedAnnotations(handler string) ([]string, error)
func (r *Runtime) RuntimeType(runtimeHandler string) (string, error)
func (r *Runtime) Seccomp(handler string) (*seccomp.Config, error)
func (r *Runtime) Timezone() string
func (r *Runtime) GetContainerMinMemory(runtimeHandler string) (int64, error)
func (r *Runtime) RuntimeSupportsIDMap(runtimeHandler string) bool
func (r *Runtime) RuntimeSupportsRROMounts(runtimeHandler string) bool
func (r *Runtime) RuntimeDefaultAnnotations(runtimeHandler string) (map[string]string, error)
func (r *Runtime) RuntimeStreamWebsockets(runtimeHandler string) (bool, error)
```

---

### Runtime Container Lifecycle

```go
func (r *Runtime) CreateContainer(ctx context.Context, c *Container, cgroupParent string, restore bool) error
func (r *Runtime) StartContainer(ctx context.Context, c *Container) error
func (r *Runtime) StopContainer(ctx context.Context, c *Container, timeout int64) error
func (r *Runtime) DeleteContainer(ctx context.Context, c *Container) error
func (r *Runtime) UpdateContainer(ctx context.Context, c *Container, res *rspec.LinuxResources) error
func (r *Runtime) UpdateContainerStatus(ctx context.Context, c *Container) error
func (r *Runtime) PauseContainer(ctx context.Context, c *Container) error
func (r *Runtime) UnpauseContainer(ctx context.Context, c *Container) error
```

Each method resolves the appropriate `RuntimeImpl` for the container and
delegates to it.

---

### Runtime Stats and I/O

```go
func (r *Runtime) ContainerStats(ctx context.Context, c *Container, cgroup string) (*stats.CgroupStats, error)
func (r *Runtime) DiskStats(ctx context.Context, c *Container, cgroup string) (*stats.DiskStats, error)
func (r *Runtime) ExecContainer(ctx context.Context, c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error
func (r *Runtime) ExecSyncContainer(ctx context.Context, c *Container, command []string, timeout int64) (*types.ExecSyncResponse, error)
func (r *Runtime) AttachContainer(ctx context.Context, c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error
func (r *Runtime) PortForwardContainer(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser) error
func (r *Runtime) ReopenContainerLog(ctx context.Context, c *Container) error
```

---

### Runtime Checkpoint/Restore

```go
func (r *Runtime) CheckpointContainer(ctx context.Context, c *Container, specgen *rspec.Spec, leaveRunning bool) error
func (r *Runtime) RestoreContainer(ctx context.Context, c *Container, cgroupParent, mountLabel string) error
```

---

### Container

#### `NewContainer`

Creates a new runtime container object with all metadata.

```go
func NewContainer(id, name, bundlePath, logPath string,
    labels, crioAnnotations, annotations map[string]string,
    userRequestedImage string,
    someNameOfTheImage *references.RegistryImageReference,
    imageID *storage.StorageImageID,
    someRepoDigest string,
    md *types.ContainerMetadata,
    sandbox string,
    terminal, stdin, stdinOnce bool,
    runtimeHandler, dir string,
    created time.Time,
    stopSignal string,
) (*Container, error)
```

---

#### `NewSpoofedContainer`

Creates a lightweight container placeholder without full metadata. Used
during restore when the full image data is not yet available.

```go
func NewSpoofedContainer(id, name string, labels map[string]string, sandbox string, created time.Time, dir string) *Container
```

---

The `Container` struct exposes many accessor methods. Key ones:

| Method           | Returns                     | Description                        |
| ---------------- | --------------------------- | ---------------------------------- |
| `ID()`           | `string`                    | Container ID                       |
| `Name()`         | `string`                    | Container name                     |
| `BundlePath()`   | `string`                    | OCI bundle directory               |
| `LogPath()`      | `string`                    | Container log file path            |
| `State()`        | `*ContainerState`           | Current container state            |
| `Spec()`         | `specs.Spec`                | OCI runtime spec copy              |
| `Sandbox()`      | `string`                    | Parent sandbox ID                  |
| `Pid()`          | `(int, error)`              | Container init PID                 |
| `Spoofed()`      | `bool`                      | Whether container is a placeholder |
| `Created()`      | `bool`                      | Whether `SetCreated` was called    |
| `Volumes()`      | `[]ContainerVolume`         | Bind-mounted volumes               |
| `MountPoint()`   | `string`                    | Rootfs mount point                 |
| `GetResources()` | `*types.ContainerResources` | Linux resource constraints         |

---

### Helper Functions

#### `TruncateAndReadFile`

Reads a file, truncating it if larger than the specified size.

```go
func TruncateAndReadFile(ctx context.Context, path string, size int64) ([]byte, error)
```

---

#### `ReadConmonPidFile`

Reads conmon's PID from its pid file for a container.

```go
func ReadConmonPidFile(c *Container) (int, error)
```

---

#### `EncodeKataVirtualVolumeToBase64`

Encodes a Kata virtual volume definition to base64 for annotation
injection.

```go
func EncodeKataVirtualVolumeToBase64(ctx context.Context, volume *katavolume.KataVirtualVolume) (string, error)
```

---

## Package `internal/storage`

### ImageServer

The `ImageServer` interface wraps image operations against
containers/storage.

```go
type ImageServer interface {
    ListImages(systemContext *types.SystemContext) ([]ImageResult, error)
    ImageStatusByID(systemContext *types.SystemContext, id StorageImageID) (*ImageResult, error)
    ImageStatusByName(systemContext *types.SystemContext, name RegistryImageReference) (*ImageResult, error)
    PullImage(ctx context.Context, imageName RegistryImageReference, options *ImageCopyOptions) (RegistryImageReference, error)
    DeleteImage(systemContext *types.SystemContext, id StorageImageID) error
    UntagImage(systemContext *types.SystemContext, name RegistryImageReference) error
    GetStore() storage.Store
    HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput string) *StorageImageID
    CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]RegistryImageReference, error)
    UpdatePinnedImagesList(imageList []string)
    IsRunningImageAllowed(ctx context.Context, systemContext *types.SystemContext, userSpecifiedImage RegistryImageReference, imageID StorageImageID) error
}
```

#### `GetImageService`

```go
func GetImageService(ctx context.Context, store storage.Store, storageTransport StorageTransport, serverConfig *config.Config) (ImageServer, error)
```

---

### RuntimeServer

The `RuntimeServer` interface wraps storage operations for container and
sandbox lifecycle.

```go
type RuntimeServer interface {
    CreatePodSandbox(systemContext *types.SystemContext, podName, podID string, pauseImage RegistryImageReference, imageAuthFile, containerName, metadataName, uid, namespace string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (ContainerInfo, error)
    CreateContainer(systemContext *types.SystemContext, podName, podID, userRequestedImage string, imageID StorageImageID, containerName, containerID, metadataName string, attempt uint32, idMappingsOptions *storage.IDMappingOptions, labelOptions []string, privileged bool) (ContainerInfo, error)
    DeleteContainer(ctx context.Context, idOrName string) error
    StartContainer(idOrName string) (string, error)
    StopContainer(ctx context.Context, idOrName string) error
    GetContainerMetadata(idOrName string) (RuntimeContainerMetadata, error)
    SetContainerMetadata(idOrName string, metadata *RuntimeContainerMetadata) error
    GetWorkDir(id string) (string, error)
    GetRunDir(id string) (string, error)
}
```

`StartContainer` mounts the container's rootfs and returns the mount point.
`StopContainer` unmounts it.

#### `GetRuntimeService`

```go
func GetRuntimeService(ctx context.Context, storageImageServer ImageServer, storageTransport StorageTransport) RuntimeServer
```

---

### Package-Level Functions

```go
func WrapSignatureCRIErrorIfNeeded(err error) error
func FilterPinnedImage(image string, pinnedImages []*regexp.Regexp) bool
func CompileRegexpsForPinnedImages(patterns []string) []*regexp.Regexp
```

---

## Package `internal/factory/container`

The container factory builds OCI runtime specs from CRI container
configuration.

#### `New`

```go
func New() (Container, error)
```

The returned `Container` interface provides spec-building methods:

| Method                               | Purpose                               |
| ------------------------------------ | ------------------------------------- |
| `SetConfig`                          | Set CRI container and sandbox configs |
| `SetNameAndID`                       | Generate container name and ID        |
| `SetPrivileged`                      | Configure privileged mode             |
| `Spec`                               | Access the OCI spec generator         |
| `SpecAddMount`                       | Add a bind mount to the spec          |
| `SpecAddDevices`                     | Add devices to the spec               |
| `SpecInjectCDIDevices`               | Inject CDI devices                    |
| `SpecAddAnnotations`                 | Add OCI annotations                   |
| `SpecAddNamespaces`                  | Configure namespace sharing           |
| `SpecSetupCapabilities`              | Set Linux capabilities                |
| `SpecSetPrivileges`                  | Configure security context            |
| `SpecSetLinuxContainerResources`     | Set CPU/memory limits                 |
| `SpecSetProcessArgs`                 | Set entrypoint and command            |
| `AddUnifiedResourcesFromAnnotations` | Apply unified cgroup resources        |
| `PidNamespace`                       | Return the PID namespace              |
| `WillRunSystemd`                     | Whether the container runs systemd    |

---

## Package `internal/runtimehandlerhooks`

Runtime handler hooks run at specific points in the container lifecycle to
apply runtime-specific behavior (e.g. CPU pinning for high-performance
workloads).

#### `RuntimeHandlerHooks` Interface

```go
type RuntimeHandlerHooks interface {
    PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, c *oci.Container) error
    PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
    PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
    PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}
```

#### `NewHooksRetriever`

```go
func NewHooksRetriever(ctx context.Context, config *libconfig.Config) *HooksRetriever
```

#### `Get`

Returns the hooks for a given runtime handler and sandbox annotations.

```go
func (hr *HooksRetriever) Get(ctx context.Context, runtimeName string, sandboxAnnotations map[string]string) RuntimeHandlerHooks
```

#### `RestoreIrqBalanceConfig`

Restores IRQ balance configuration after high-performance hook cleanup.

```go
func RestoreIrqBalanceConfig(ctx context.Context, irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityProcFile string) error
```

---

## Package `internal/hostport`

Manages host-to-container port mappings using iptables or nftables.

#### `HostPortManager` Interface

```go
type HostPortManager interface {
    Add(id, name, podIP string, hostportMappings []*PortMapping) error
    Remove(id string, hostportMappings []*PortMapping) error
}
```

#### Constructors

```go
func NewMetaHostportManager(ctx context.Context) (HostPortManager, error)
func NewNoopHostportManager() HostPortManager
```

`NewMetaHostportManager` auto-detects whether to use iptables or nftables.

---

## Package `internal/config` Subsystems

### seccomp

```go
func New() *Config
func DefaultProfile() *seccomp.Seccomp
func (c *Config) LoadProfile(profilePath string) error
func (c *Config) LoadDefaultProfile() error
func (c *Config) IsDisabled() bool
func (c *Config) Profile() *seccomp.Seccomp
func (c *Config) Setup(ctx context.Context, sys *imagetypes.SystemContext, msgChan chan Notification, containerID, containerName string, sandboxAnnotations, imageAnnotations map[string]string, specGenerator *generate.Generator, profileField *types.SecurityProfile, graphRoot string) (*Notifier, string, error)
func NewNotifier(ctx context.Context, msgChan chan Notification, containerID, listenerPath string, annotationMap map[string]string) (*Notifier, error)
```

### cgmgr

```go
func New() CgroupManager
func SetCgroupManager(cgroupManager string) (CgroupManager, error)
func VerifyMemoryIsEnough(memoryLimit, minMemory int64) error
func MoveProcessToContainerCgroup(containerPid, commandPid int) error
func LibctrManager(cgroup, parent string, systemd bool) (cgroups.Manager, error)
```

### cnimgr

```go
func New(defaultNetwork, networkDir string, pluginDirs ...string) (*CNIManager, error)
func (c *CNIManager) ReadyOrError() error
func (c *CNIManager) Plugin() ocicni.CNIPlugin
func (c *CNIManager) AddWatcher() chan bool
func (c *CNIManager) Shutdown()
func (c *CNIManager) GC(ctx context.Context, validPodList PodNetworkLister) error
```

### conmonmgr

```go
func New(conmonPath string) (*ConmonManager, error)
func (c *ConmonManager) SupportsLogGlobalSizeMax() bool
func (c *ConmonManager) SupportsSync() bool
```

### device

```go
func New() *Config
func (d *Config) LoadDevices(devsFromConfig []string) error
func (d *Config) Devices() []Device
func DevicesFromAnnotation(annotation string, allowedDevices []string) ([]Device, error)
```

### nsmgr

```go
func New(namespacesDir, pinnsPath string) *NamespaceManager
func (mgr *NamespaceManager) Initialize() error
func (mgr *NamespaceManager) NewPodNamespaces(cfg *PodNamespacesConfig) ([]Namespace, error)
func (mgr *NamespaceManager) NamespaceFromProcEntry(pid int, nsType NSType) (Namespace, error)
func GetNamespace(nsPath string, nsType NSType) (Namespace, error)
func NamespacePathFromProc(nsType NSType, pid int) string
```

### node

```go
func ValidateConfig() error
func CgroupIsV2() bool
func CgroupHasMemorySwap() bool
func CgroupHasHugetlb() bool
func CgroupHasPid() bool
func SystemdHasAllowedCPUs() bool
```

### rdt

```go
func New() *Config
func (c *Config) Supported() bool
func (c *Config) Enabled() bool
func (c *Config) Load(path string) error
func (c *Config) ContainerClassFromAnnotations(containerName string, containerAnnotations, podAnnotations map[string]string) (string, error)
```

### apparmor

```go
func New() *Config
func (c *Config) LoadProfile(profile string) error
func (c *Config) IsEnabled() bool
func (c *Config) Apply(p *runtimeapi.LinuxContainerSecurityContext) (string, error)
```

### blockio

```go
func New() *Config
func (c *Config) Enabled() bool
func (c *Config) Load(path string) error
func (c *Config) Reload() error
func (c *Config) SetReload(reload bool)
func (c *Config) ReloadRequired() bool
```

### capabilities

```go
func Default() Capabilities
func (c Capabilities) Validate() error
```

### nri

```go
func New() *Config
func (c *Config) Validate(onExecution bool) error
func (c *Config) WithTracing(enable bool) *Config
func (c *Config) ToOptions() []nri.Option
func (c *Config) ConfigureTimeouts()
```

### ulimits

```go
func New() *Config
func (c *Config) LoadUlimits(ulimits []string) error
func (c *Config) Ulimits() []Ulimit
```

---

## Package `utils`

### ID and User Utilities

```go
func GenerateID() (string, error)
func GetUserInfo(rootfs, userName string) (uid, gid uint32, additionalGids []uint32, _ error)
func GeneratePasswd(username string, uid, gid uint32, homedir, rootfs, rundir string) (string, error)
func GenerateGroup(gid uint32, rootfs, rundir string) (string, error)
func GetGroup(containerMount, groupIDorName string) (*user.Group, error)
func GetUser(containerMount, userIDorName string) (*user.User, error)
func StatusToExitCode(status int) int
func Int32Ptr(i int32) *int32
```

### Filesystem Utilities

```go
func EnsureSaneLogPath(logPath string) error
func SyncParent(path string) error
func Sync(path string) error
func Syncfs(path string) error
func GetDiskUsageStats(path string) (dirSize, inodeCount uint64, _ error)
func IsDirectory(path string) error
```

### I/O and Signal Utilities

```go
func CopyDetachable(dst io.Writer, src io.Reader, keys []byte) (int64, error)
func HandleResizing(resize <-chan remotecommand.TerminalSize, resizeFunc func(size remotecommand.TerminalSize))
func WriteGoroutineStacksToFile(path string) error
func WriteGoroutineStacksTo(f io.Writer) error
func RunUnderSystemdScope(pid int, slice, unit string, properties ...systemdDbus.Property) error
```

### Duration Parsing

```go
func ParseDuration(s string) (time.Duration, error)
```

Parses human-readable durations (e.g. `"24h"`) or string-encoded integer
seconds. Negative values are converted to positive.

---

## Package `utils/errdefs`

Error sentinels and gRPC error mapping.

**Error Sentinels:**

| Variable                | gRPC Code            | Meaning                 |
| ----------------------- | -------------------- | ----------------------- |
| `ErrUnknown`            | `Unknown`            | Unknown error           |
| `ErrInvalidArgument`    | `InvalidArgument`    | Invalid argument        |
| `ErrNotFound`           | `NotFound`           | Resource not found      |
| `ErrAlreadyExists`      | `AlreadyExists`      | Resource already exists |
| `ErrFailedPrecondition` | `FailedPrecondition` | Precondition not met    |
| `ErrUnavailable`        | `Unavailable`        | Service unavailable     |
| `ErrNotImplemented`     | `Unimplemented`      | Not implemented         |

**Type Checkers:**

```go
func IsInvalidArgument(err error) bool
func IsNotFound(err error) bool
func IsAlreadyExists(err error) bool
func IsFailedPrecondition(err error) bool
func IsUnavailable(err error) bool
func IsNotImplemented(err error) bool
```

**gRPC Mapping:**

```go
func ToGRPC(err error) error
func ToGRPCf(err error, format string, args ...any) error
func FromGRPC(err error) error
```

---

## Package `utils/cmdrunner`

Command execution abstraction with optional command prepending (useful for
running commands inside mount namespaces).

```go
func Command(cmd string, args ...string) *exec.Cmd
func CommandContext(ctx context.Context, cmd string, args ...string) *exec.Cmd
func CombinedOutput(command string, args ...string) ([]byte, error)
func PrependCommandsWith(prependCmd string, prependArgs ...string)
func GetPrependedCmd() string
func ResetPrependedCmd()
```

The `CommandRunner` interface allows replacing the default implementation
for testing:

```go
type CommandRunner interface {
    Command(string, ...string) *exec.Cmd
    CommandContext(context.Context, string, ...string) *exec.Cmd
    CombinedOutput(string, ...string) ([]byte, error)
}
```
