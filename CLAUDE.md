# CLAUDE.md — macOS-vz-kubelet

## Kubernetes-Native macOS VM Orchestration via Apple Virtualization.framework

This is the development guide for extending Agoda's macOS-vz-kubelet with Tart image support and VM lifecycle improvements.

---

## Project Scope

The macOS-vz-kubelet transforms macOS hosts into Kubernetes nodes using Apple's Virtualization.framework, enabling native macOS VM orchestration with near-bare-metal performance.

**This repo focuses exclusively on VM orchestration:**

- Pulling and running macOS VM images as Kubernetes pods
- OCI image format support (native ORAS + Tart/Cirrus Labs)
- VM lifecycle management (start, stop, exec, logs)
- Resource management and scheduling

**Out of scope (separate repos):**

| Concern | Repo |
|---------|------|
| iOS/Android device brokering | `silikube-device-broker` |
| Mac host fleet management | `silikube-fleet-agent` |

---

## Core Constraints

| Constraint | Implication |
|------------|-------------|
| 2 macOS VMs per host max | Apple Virtualization.framework hard limit |
| VMNet requires Apple entitlement | NAT networking for MVP (sufficient for CI/CD) |
| Apple Silicon only | No x86 macOS VMs |

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│  Kubernetes Control Plane                                                 │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │  Standard K8s: Pods, Services, ConfigMaps, Secrets                  │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────────┘
                                     │ kubelet API
                                     ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  Mac Host (Apple Silicon)                                                 │
│                                                                           │
│  ┌──────────────────────────────────────────────────────────────────────┐│
│  │  macOS-vz-kubelet (Virtual Kubelet)                                  ││
│  │                                                                       ││
│  │  ┌────────────────────┐  ┌────────────────────┐  ┌─────────────────┐ ││
│  │  │  OCI Store         │  │  VM Provider       │  │  Pod Lifecycle  │ ││
│  │  │                    │  │                    │  │                 │ ││
│  │  │  • ORAS pull       │  │  • Virtualization  │  │  • Create/Start │ ││
│  │  │  • Tart LZ4        │  │    .framework      │  │  • Stop/Delete  │ ││
│  │  │  • Image cache     │  │  • NAT networking  │  │  • Exec/Logs    │ ││
│  │  │  • Format detect   │  │  • Disk overlays   │  │  • Health check │ ││
│  │  └────────────────────┘  └────────────────────┘  └─────────────────┘ ││
│  └──────────────────────────────────────────────────────────────────────┘│
│                                                                           │
│  ┌──────────────────────────────────────────────────────────────────────┐│
│  │  Virtualization Layer (Apple Virtualization.framework)               ││
│  │                                                                       ││
│  │  ┌─────────────────────────────┐  ┌─────────────────────────────┐   ││
│  │  │  macOS VM 1 (K8s Pod)       │  │  macOS VM 2 (K8s Pod)       │   ││
│  │  │  - Xcode builds             │  │  - iOS Simulator tests      │   ││
│  │  └─────────────────────────────┘  └─────────────────────────────┘   ││
│  └──────────────────────────────────────────────────────────────────────┘│
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Architecture Decisions

### Decision 1: Native Tart image support via LZ4 decompression

**Chosen approach:** Extend the existing OCI store to natively support Tart images by adding LZ4 decompression and Tart media type handling. No external Tart CLI dependency.

**Rationale:** Tart images are standard OCI artifacts hosted on ghcr.io. The only differences are compression format and media types:

| Aspect | vz-kubelet (ORAS) | Tart (Cirrus Labs) |
|--------|-------------------|---------------------|
| Compression | GZIP (pgzip) | LZ4 (chunked frames) |
| Layer structure | Single compressed blob | Multiple LZ4 layers |
| Config location | `manifest.Config` | `layers[0]` |

Both produce identical VM artifacts (disk.img, nvram.bin). By extending the existing ORAS-based pipeline with LZ4 support, we avoid runtime dependencies and maintain a single-binary deployment.

### Decision 2: NAT networking for MVP

**Chosen approach:** NAT mode via `VZNATNetworkDeviceAttachment`.

**Rationale:** VMNet entitlements require Apple approval (2-3+ weeks). NAT is sufficient for CI/CD:

- ✅ Internet access for builds
- ✅ Host-to-VM SSH via `kubectl exec`
- ✅ iOS Simulator testing
- ❌ No inbound external connections (acceptable for MVP)

### Decision 3: APFS copy-on-write for fast VM cloning

**Chosen approach:** Use macOS `clonefile()` syscall for instant disk image copies.

**Rationale:** Base images are 50-80GB. Full copies take minutes. APFS CoW clones are instant and only consume space for deltas.

---

## Implementation Roadmap

| Phase | Focus | Key Deliverables | Status |
|-------|-------|------------------|--------|
| 1 | Tart image support | Tart media type detection, multi-layer disk assembly, config parsing | ✅ Done |
| 1.5 | OCI Layout compliance | Resumable downloads, standard blob storage, cache persistence | 📋 Planned |
| 2 | Enhanced VM lifecycle | APFS CoW overlays, graceful shutdown, Prometheus metrics | 📋 Planned |
| 3 | Production hardening | Health checks, resource limits, error recovery | 📋 Planned |

---

## Phase 1: Native Tart Image Support ✅

**Status:** Completed

**Key findings:**
- Tart layers in OCI registries are stored as **raw, uncompressed disk chunks** (not LZ4)
- Each layer starts with `bv41` magic number (Tart's chunk format)
- NVRAM and config layers are also uncompressed
- No LZ4 dependency needed

**Implementation:**
- Tart media type detection (`application/vnd.cirruslabs.tart.*`)
- Sequential layer concatenation for disk assembly
- Parse Tart config from `layers[0]` (not `manifest.Config`)
- Map Tart config to internal VMImage format
- Support for both ORAS and Tart image formats in unified Store

**Exit criteria:** ✅ `ghcr.io/cirruslabs/macos-sequoia-xcode:16` pulls and boots successfully.

---

## Phase 1.5: OCI Layout Compliance & Resumable Downloads

**Problem:** Current implementation uses temp files with in-memory digest mapping. On restart, all download progress is lost and images must be re-downloaded (100GB+ for full Xcode images).

**Solution:** Adopt OCI Image Layout specification for standard-compliant blob storage.

### Architecture Changes

```
Before (temp files):
/var/folders/.../T/
  macosvz_file_1234  (lost on restart)
  macosvz_file_5678  (lost on restart)

After (OCI layout):
~/Library/Caches/com.agoda.fleet.virtualization/blobs/
  sha256/
    abc123def...  (permanent, validated)
    456789ghi...  (permanent, validated)
  oci-layout      (version marker)
```

### Implementation

**1. Store refactor (pkg/oci/store.go):**
```go
type Store struct {
    blobsDir string  // e.g., blobs/sha256/
}

func (s *Store) Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error {
    blobPath := filepath.Join(s.blobsDir, expected.Digest.Algorithm().String(),
                              expected.Digest.Encoded())

    // Check if already exists (enables resume)
    if s.validateBlob(blobPath, expected) {
        return nil  // Skip download
    }

    // Atomic write (temp + validate + rename)
    tmpPath := blobPath + ".partial"
    // Write with digest validation...
    os.Rename(tmpPath, blobPath)
}

func (s *Store) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
    blobPath := filepath.Join(s.blobsDir, desc.Digest.Algorithm().String(),
                              desc.Digest.Encoded())
    return os.Open(blobPath)
}

func (s *Store) Exists(desc ocispec.Descriptor) bool {
    blobPath := filepath.Join(s.blobsDir, desc.Digest.Algorithm().String(),
                              desc.Digest.Encoded())
    return s.validateBlob(blobPath, desc)
}
```

**2. Migration path:**
- Detect legacy temp file cache
- Provide migration tool or warning
- Support both formats during transition

**3. Benefits:**
- ✅ **Resumable downloads**: ORAS checks `Exists()` before downloading each layer
- ✅ **Survives restarts**: Blobs persist across crashes/restarts
- ✅ **Standard compliance**: Compatible with OCI tooling (skopeo, crane, etc.)
- ✅ **Cache pre-warming**: Can use external tools to populate cache
- ✅ **Atomic operations**: `.partial` files indicate incomplete downloads
- ✅ **Digest validation**: Every blob validated on write

**4. Testing:**
- Interrupt download mid-pull, verify resume
- Restart kubelet during download, verify resume
- Corrupt blob file, verify re-download
- Performance: measure cache hit speedup

**Exit criteria:**
- Download can be interrupted and resumed without re-downloading
- 100GB+ images can be pulled reliably
- Compatible with OCI ecosystem tools

**Effort estimate:** 4-6 days

---

---

## Phase 2: Enhanced VM Lifecycle

### APFS Copy-on-Write Overlays

```go
// pkg/vm/overlay.go
/*
#include <sys/clonefile.h>
*/
import "C"

func CreateOverlay(basePath, overlayPath string) error {
    ret := C.clonefile(
        C.CString(basePath),
        C.CString(overlayPath),
        C.CLONE_NOFOLLOW,
    )
    if ret != 0 {
        return copyFile(basePath, overlayPath) // Fallback for non-APFS
    }
    return nil
}
```

### Graceful VM Shutdown

```go
func (vm *Instance) GracefulStop(ctx context.Context, timeout time.Duration) error {
    // Send ACPI power button
    vm.VirtualMachine.RequestStop()

    deadline := time.After(timeout)
    for {
        select {
        case <-deadline:
            return vm.VirtualMachine.Stop() // Force stop
        default:
            if vm.VirtualMachine.State() == vz.VirtualMachineStateStopped {
                return nil
            }
            time.Sleep(500 * time.Millisecond)
        }
    }
}
```

### Prometheus Metrics

```go
var (
    VMsRunning = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "vz_kubelet_vms_running",
        Help: "Number of currently running VMs",
    })

    ImagePullDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "vz_kubelet_image_pull_seconds",
        Help:    "Time to pull and extract VM images",
        Buckets: prometheus.ExponentialBuckets(10, 2, 10),
    }, []string{"format"}) // "oras" or "tart"

    VMBootDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "vz_kubelet_vm_boot_seconds",
        Help:    "Time from VM start to SSH ready",
        Buckets: prometheus.ExponentialBuckets(5, 2, 8),
    })
)
```

---

## Development Environment Setup

```bash
# Prerequisites
brew install go kubectl

# Clone and build
git clone https://github.com/agoda-com/macOS-vz-kubelet.git
cd macOS-vz-kubelet
go build -o bin/vz-kubelet ./cmd/vz-kubelet

# Sign with entitlements (required for Virtualization.framework)
codesign --entitlements resources/vz.entitlements -s - bin/vz-kubelet

# Run locally
export KUBECONFIG=~/.kube/config
./bin/vz-kubelet --nodename=$(hostname)
```

---

## Testing

```bash
# Unit tests
go test ./pkg/...

# Integration: Pull Tart image
kubectl apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: test-tart
spec:
  nodeName: $(hostname)
  containers:
  - name: macos
    image: ghcr.io/cirruslabs/macos-sequoia-xcode:16
EOF

kubectl wait --for=condition=Ready pod/test-tart --timeout=600s
kubectl exec test-tart -- sw_vers
```

---

## Key File Locations

| Path | Purpose |
|------|---------|
| `pkg/oci/` | OCI image store, media types, decompression |
| `pkg/oci/lz4.go` | LZ4 decompression for Tart images |
| `pkg/oci/tart.go` | Tart manifest/config processing |
| `pkg/vm/` | VM lifecycle management |
| `pkg/vm/overlay.go` | APFS copy-on-write cloning |
| `pkg/downloader/` | Image pull orchestration |
| `cmd/vz-kubelet/` | Main binary entrypoint |

---

## Implementation Learnings

### Tart Image Format Reality vs. Documentation

**Issue:** Initial implementation attempted LZ4 decompression of Tart disk layers, resulting in deadlock.

**Root cause:** Despite Tart's historical use of LZ4 compression, layers in ghcr.io OCI registries are stored as **raw, uncompressed disk chunks**.

**Evidence:**
```bash
$ hexdump -C layer.blob | head -3
00000000  62 76 34 31 00 00 01 00  # "bv41" magic number
00000010  01 53 00 00 c0 99 88 77  # Tart chunk header
```

**Correct implementation:**
- Disk layers: Sequential concatenation (no decompression)
- NVRAM layer: Direct copy (no decompression)
- Config layer: JSON parse (no decompression)

**Debugging technique:**
```bash
# Test LZ4 decompression on suspected layer
echo 'package main
import ("github.com/pierrec/lz4/v4"; "io"; "os")
func main() {
    f, _ := os.Open(os.Args[1])
    r := lz4.NewReader(f)
    io.Copy(os.Stdout, r)  // Will fail with "bad magic number" if not LZ4
}' > test.go
go run test.go layer.blob
```

**Takeaway:** Always validate compression format with hex inspection before implementing decompression logic. Media type alone is insufficient.

---

## FAQ

**Q: Can I run more than 2 VMs per Mac?**
A: No. Apple hard limit in Virtualization.framework. Consider Anka (commercial) for higher density claims, though this is unverified.

**Q: Do I need Apple entitlements for CI/CD?**
A: No. NAT networking works without entitlements and covers most CI/CD use cases.

**Q: What about iOS device testing?**
A: Physical iOS devices are managed by `silikube-device-broker`, a separate repo. iOS *Simulators* run inside the macOS VMs and work out of the box.

**Q: How do I manage the Mac hosts themselves?**
A: Host management (OS updates, Xcode installs, reboots) is handled by `silikube-fleet-agent`, a separate repo.
