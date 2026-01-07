package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
)

// HandleManifest processes format-specific manifest handling after ORAS copy completes.
//
// For ORAS/vz-kubelet format: Processing happens during Push() as layers arrive.
// For Tart format: We need all layers cached before sequential concatenation.
//
// This method is called by the downloader after oras.Copy() completes.
func (s *Store) HandleManifest(ctx context.Context, manifest ocispec.Manifest) error {
	logger := log.G(ctx)
	logger.Infof("HandleManifest called: %d layers, config type: %s", len(manifest.Layers), manifest.Config.MediaType)

	if len(manifest.Layers) > 0 {
		logger.Infof("First layer media type: %s", manifest.Layers[0].MediaType)
	}

	isTart := IsTartManifest(manifest)
	logger.Infof("IsTartManifest detection: %v", isTart)

	if isTart {
		logger.Info("Processing as Tart manifest...")
		if err := s.processTartManifest(ctx, manifest); err != nil {
			logger.Errorf("Failed to process Tart manifest: %v", err)
			return fmt.Errorf("process tart manifest: %w", err)
		}
		logger.Info("Tart manifest processing completed successfully")
		return nil
	}

	// ORAS format already processed during Push()
	logger.Info("Not a Tart manifest, assuming ORAS format (processed during Push)")
	return nil
}

// processTartManifest handles a Tart OCI manifest, separating disk layers
// from config and NVRAM, then decompressing appropriately.
//
// IMPORTANT: Tart config is in layers[0], NOT manifest.Config!
// Layer structure: [config, disk_chunk_1, disk_chunk_2, ..., nvram]
func (s *Store) processTartManifest(ctx context.Context, manifest ocispec.Manifest) (err error) {
	ctx, span := trace.StartSpan(ctx, "OCI.processTartManifest")
	defer func() {
		span.SetStatus(err)
		span.End()
	}()
	logger := log.G(ctx)

	logger.Infof("Starting Tart manifest processing with %d total layers", len(manifest.Layers))

	// Categorize layers by type
	configLayer, diskLayers, nvramLayer := CategorizeTartLayers(manifest)

	logger.Infof("Layer categorization: config=%v, disk layers=%d, nvram=%v",
		configLayer != nil, len(diskLayers), nvramLayer != nil)

	// Validate required layers
	if err := ValidateTartLayers(configLayer, diskLayers, nvramLayer); err != nil {
		logger.Errorf("Layer validation failed: %v", err)
		return fmt.Errorf("validate tart layers: %w", err)
	}

	logger.Infof("Processing Tart manifest: %d disk layers", len(diskLayers))

	// Ensure working directory exists
	logger.Infof("Ensuring working directory exists: %s", s.workingDir)
	if err := os.MkdirAll(s.workingDir, os.ModePerm); err != nil {
		logger.Errorf("Failed to create working directory: %v", err)
		return fmt.Errorf("failed to ensure the working directory exists: %w", err)
	}

	// Process disk layers (sequential concatenation, NOT LZ4 decompression)
	diskPath := filepath.Join(s.workingDir, "disk.img")
	logger.Infof("Assembling disk image to: %s", diskPath)
	if err := s.DecompressTartDiskLayers(ctx, diskLayers, diskPath); err != nil {
		logger.Errorf("Failed to assemble disk: %v", err)
		return fmt.Errorf("decompress tart disk: %w", err)
	}
	s.mediaTypeToPath.Store(string(MediaTypeDiskImage), diskPath)
	logger.Infof("Disk image assembled successfully: %s", diskPath)

	// Process NVRAM layer (direct copy, NOT LZ4 decompression)
	nvramPath := filepath.Join(s.workingDir, "nvram.bin")
	logger.Infof("Processing NVRAM to: %s", nvramPath)
	if err := s.DecompressSingleLZ4Layer(ctx, *nvramLayer, nvramPath); err != nil {
		logger.Errorf("Failed to process NVRAM: %v", err)
		return fmt.Errorf("decompress tart nvram: %w", err)
	}
	// Map to our internal media type for compatibility
	s.mediaTypeToPath.Store(string(MediaTypeAuxImage), nvramPath)
	logger.Infof("NVRAM processed successfully: %s", nvramPath)

	// Process config (from layers[0], not manifest.Config)
	logger.Info("Processing Tart config layer...")
	if err := s.processTartConfig(ctx, *configLayer); err != nil {
		logger.Errorf("Failed to process config: %v", err)
		return fmt.Errorf("process tart config: %w", err)
	}
	logger.Info("Tart config processed successfully")

	logger.Info("Tart manifest processing completed - all artifacts ready")
	return nil
}

// DecompressTartDiskLayers handles Tart's multi-layer disk format.
//
// Tart layers use Apple's Compression framework with LZ4 (bv41 format).
// We use a Swift helper binary to decompress each layer, as Go's LZ4 libraries
// don't support Apple's proprietary bv41 wrapper format with dictionary chaining.
//
// Reference: https://github.com/cirruslabs/tart/blob/main/Sources/tart/OCI/Layerizer/DiskV2.swift
//
// Flow:
//  1. ORAS Copy() downloads layers in parallel → Store cache
//  2. For each layer, save to temp file and decompress via Swift helper
//  3. Append decompressed data to final disk image
func (s *Store) DecompressTartDiskLayers(ctx context.Context, layers []ocispec.Descriptor, destPath string) (err error) {
	ctx, span := trace.StartSpan(ctx, "OCI.DecompressTartDiskLayers")
	defer func() {
		span.SetStatus(err)
		span.End()
	}()
	logger := log.G(ctx)

	if len(layers) == 0 {
		return fmt.Errorf("no disk layers provided")
	}

	// Calculate total compressed size for progress logging
	var totalSize int64
	for _, layer := range layers {
		totalSize += layer.Size
	}
	logger.Infof("Decompressing Tart disk: %d layers, %d MB compressed (Apple Compression format)",
		len(layers), totalSize/1024/1024)

	// Create destination disk file
	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("create disk file: %w", err)
	}
	defer func() {
		if cerr := dest.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	var totalDecompressed int64
	var bytesRead int64

	// Process each layer sequentially
	for i, layer := range layers {
		logger.Debugf("Decompressing layer %d/%d (%s)", i+1, len(layers), layer.Digest.String()[:12])

		// Decompress layer using Swift helper
		decompressed, err := s.decompressTartLayer(ctx, layer)
		if err != nil {
			return fmt.Errorf("decompress layer %d: %w", i, err)
		}

		// Append to disk
		n, err := dest.Write(decompressed)
		if err != nil {
			return fmt.Errorf("write layer %d: %w", i, err)
		}

		totalDecompressed += int64(n)
		bytesRead += layer.Size

		// Progress logging (every 10 layers)
		if (i+1)%10 == 0 || i == len(layers)-1 {
			pct := float64(bytesRead) / float64(totalSize) * 100
			logger.Infof("Progress: layer %d/%d complete (%.1f%%, %d MB → %d MB)",
				i+1, len(layers), pct, bytesRead/1024/1024, totalDecompressed/1024/1024)
		}
	}

	if err := dest.Sync(); err != nil {
		return fmt.Errorf("sync disk file: %w", err)
	}

	logger.Infof("Tart disk assembly complete: %d MB compressed → %d MB decompressed",
		bytesRead/1024/1024, totalDecompressed/1024/1024)
	return nil
}

// decompressTartLayer decompresses a single Tart layer using the Swift helper.
// Returns the decompressed data in memory.
func (s *Store) decompressTartLayer(ctx context.Context, layer ocispec.Descriptor) ([]byte, error) {
	// Fetch compressed layer from cache
	rc, err := s.Fetch(ctx, layer)
	if err != nil {
		return nil, fmt.Errorf("fetch layer: %w", err)
	}
	defer rc.Close()

	// Write compressed data to temp file
	tmpCompressed, err := os.CreateTemp("", "tart-layer-*.lz4")
	if err != nil {
		return nil, fmt.Errorf("create temp compressed file: %w", err)
	}
	defer os.Remove(tmpCompressed.Name())
	defer tmpCompressed.Close()

	if _, err := io.Copy(tmpCompressed, rc); err != nil {
		return nil, fmt.Errorf("write compressed data: %w", err)
	}
	tmpCompressed.Close()

	// Create temp file for decompressed output
	tmpDecompressed, err := os.CreateTemp("", "tart-layer-*.raw")
	if err != nil {
		return nil, fmt.Errorf("create temp decompressed file: %w", err)
	}
	defer os.Remove(tmpDecompressed.Name())
	tmpDecompressed.Close()

	// Decompress using Swift helper
	helperPath := filepath.Join(filepath.Dir(os.Args[0]), "decompress-lz4")
	cmd := exec.CommandContext(ctx, helperPath, tmpCompressed.Name(), tmpDecompressed.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("decompress failed: %w (output: %s)", err, output)
	}

	// Read decompressed data
	return os.ReadFile(tmpDecompressed.Name())
}

// DecompressSingleLZ4Layer processes a single Tart auxiliary layer (e.g., NVRAM).
// Note: Tart NVRAM is stored UNCOMPRESSED in the registry (direct blob copy).
// This function just copies the blob data directly to disk.
func (s *Store) DecompressSingleLZ4Layer(ctx context.Context, layer ocispec.Descriptor, destPath string) (err error) {
	ctx, span := trace.StartSpan(ctx, "OCI.DecompressSingleLZ4Layer")
	defer func() {
		span.SetStatus(err)
		span.End()
	}()
	logger := log.G(ctx)

	rc, err := s.Fetch(ctx, layer)
	if err != nil {
		return fmt.Errorf("fetch layer: %w", err)
	}
	defer rc.Close()

	logger.Debugf("Copying uncompressed layer to %s (%d bytes)", destPath, layer.Size)

	// Create destination file
	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() {
		if cerr := dest.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Copy directly (NVRAM is not compressed)
	copied, err := io.Copy(dest, rc)
	if err != nil {
		return fmt.Errorf("copy layer: %w", err)
	}

	if err := dest.Sync(); err != nil {
		return fmt.Errorf("sync file: %w", err)
	}

	logger.Debugf("Copied %s: %d bytes", filepath.Base(destPath), copied)
	return nil
}

// processTartConfig fetches and parses the Tart config, converting it to our format.
func (s *Store) processTartConfig(ctx context.Context, configDesc ocispec.Descriptor) (err error) {
	ctx, span := trace.StartSpan(ctx, "OCI.processTartConfig")
	defer func() {
		span.SetStatus(err)
		span.End()
	}()
	logger := log.G(ctx)

	rc, err := s.Fetch(ctx, configDesc)
	if err != nil {
		return fmt.Errorf("fetch tart config: %w", err)
	}
	defer rc.Close()

	var tartConfig TartConfig
	if err := json.NewDecoder(rc).Decode(&tartConfig); err != nil {
		return fmt.Errorf("decode tart config: %w", err)
	}

	// Validate required fields
	if tartConfig.HardwareModel == "" {
		return fmt.Errorf("tart config missing hardwareModel")
	}
	if tartConfig.ECID == "" {
		return fmt.Errorf("tart config missing ecid")
	}

	logger.Debugf("Tart config: os=%s arch=%s cpu=%d mem=%dMB",
		tartConfig.OS, tartConfig.Arch, tartConfig.CPUCount, tartConfig.MemorySize/1024/1024)

	// Convert to our config format
	config := tartConfig.ToConfig()
	config.MediaType = MediaTypeConfigV1

	// Write config to disk
	configPath := filepath.Join(s.workingDir, "config.json")
	configFile, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer configFile.Close()

	encoder := json.NewEncoder(configFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(&config); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	s.mediaTypeToPath.Store(string(MediaTypeConfigV1), configPath)
	logger.Infof("Converted Tart config to %s", configPath)
	return nil
}
