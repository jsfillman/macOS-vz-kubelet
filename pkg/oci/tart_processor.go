package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pierrec/lz4/v4"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
)

// HandleManifest processes format-specific manifest handling after ORAS copy completes.
//
// For ORAS/vz-kubelet format: Processing happens during Push() as layers arrive.
// For Tart format: We need all layers cached before sequential LZ4 decompression.
//
// This method is called by the downloader after oras.Copy() completes.
func (s *Store) HandleManifest(ctx context.Context, manifest ocispec.Manifest) error {
	if IsTartManifest(manifest) {
		return s.processTartManifest(ctx, manifest)
	}
	// ORAS format already processed during Push()
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

	// Categorize layers by type
	configLayer, diskLayers, nvramLayer := CategorizeTartLayers(manifest)

	// Validate required layers
	if err := ValidateTartLayers(configLayer, diskLayers, nvramLayer); err != nil {
		return err
	}

	logger.Infof("Processing Tart manifest: %d disk layers", len(diskLayers))

	// Process disk layers (LZ4, sequential)
	diskPath := filepath.Join(s.workingDir, "disk.img")
	if err := s.DecompressTartDiskLayers(ctx, diskLayers, diskPath); err != nil {
		return fmt.Errorf("decompress tart disk: %w", err)
	}
	s.mediaTypeToPath.Store(string(MediaTypeDiskImage), diskPath)

	// Process NVRAM layer (single LZ4 layer)
	nvramPath := filepath.Join(s.workingDir, "nvram.bin")
	if err := s.DecompressSingleLZ4Layer(ctx, *nvramLayer, nvramPath); err != nil {
		return fmt.Errorf("decompress tart nvram: %w", err)
	}
	// Map to our internal media type for compatibility
	s.mediaTypeToPath.Store(string(MediaTypeAuxImage), nvramPath)
	logger.Debugf("Decompressed Tart NVRAM to %s", nvramPath)

	// Process config (from layers[0], not manifest.Config)
	if err := s.processTartConfig(ctx, *configLayer); err != nil {
		return fmt.Errorf("process tart config: %w", err)
	}

	return nil
}

// DecompressTartDiskLayers handles Tart's multi-layer LZ4 format.
//
// IMPORTANT: Layers MUST be processed sequentially because Tart's LZ4 frames
// can span layer boundaries. ORAS downloads layers in parallel and caches them
// in the Store. We then read from Store sequentially for decompression.
//
// Flow:
//  1. ORAS Copy() downloads layers in parallel → Store cache
//  2. This function calls Store.Fetch() to read cached layers (no network)
//  3. Layers are piped sequentially to LZ4 reader
//  4. LZ4 reader outputs decompressed data to disk.img
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
	var totalCompressed int64
	for _, layer := range layers {
		totalCompressed += layer.Size
	}
	logger.Infof("Decompressing Tart disk: %d layers, %d MB compressed",
		len(layers), totalCompressed/1024/1024)

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create disk file: %w", err)
	}
	defer func() {
		if cerr := dest.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Create pipe: layers write compressed data, LZ4 reader decompresses
	pr, pw := io.Pipe()
	lz4Reader := lz4.NewReader(pr)

	// Decompress in background goroutine
	var decompressErr error
	var bytesWritten int64
	done := make(chan struct{})
	go func() {
		defer close(done)
		bytesWritten, decompressErr = io.Copy(dest, lz4Reader)
	}()

	// Feed layers sequentially (order matters for LZ4 frame continuity)
	// OCI spec guarantees manifest.Layers ordering
	var bytesRead int64
	for i, layer := range layers {
		rc, err := s.Fetch(ctx, layer)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("fetch layer %d (%s): %w", i, layer.Digest, err))
			<-done
			return fmt.Errorf("fetch layer %d: %w", i, err)
		}

		n, copyErr := io.Copy(pw, rc)
		rc.Close()
		bytesRead += n

		if copyErr != nil {
			pw.CloseWithError(fmt.Errorf("copy layer %d: %w", i, copyErr))
			<-done
			return fmt.Errorf("copy layer %d: %w", i, copyErr)
		}

		// Progress logging for large images
		pct := float64(bytesRead) / float64(totalCompressed) * 100
		logger.Debugf("Decompressing: layer %d/%d complete (%.1f%%)", i+1, len(layers), pct)
	}
	pw.Close()

	// Wait for decompression to complete
	<-done
	if decompressErr != nil {
		return fmt.Errorf("decompress LZ4: %w", decompressErr)
	}

	if err := dest.Sync(); err != nil {
		return fmt.Errorf("sync disk file: %w", err)
	}

	logger.Infof("Decompressed Tart disk: %d MB uncompressed", bytesWritten/1024/1024)
	return nil
}

// DecompressSingleLZ4Layer decompresses a single LZ4-compressed layer (e.g., NVRAM).
func (s *Store) DecompressSingleLZ4Layer(ctx context.Context, layer ocispec.Descriptor, destPath string) (err error) {
	ctx, span := trace.StartSpan(ctx, "OCI.DecompressSingleLZ4Layer")
	defer func() {
		span.SetStatus(err)
		span.End()
	}()

	rc, err := s.Fetch(ctx, layer)
	if err != nil {
		return fmt.Errorf("fetch layer: %w", err)
	}
	defer rc.Close()

	dest, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() {
		if cerr := dest.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	lz4Reader := lz4.NewReader(rc)
	if _, err := io.Copy(dest, lz4Reader); err != nil {
		return fmt.Errorf("decompress LZ4: %w", err)
	}

	return dest.Sync()
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
