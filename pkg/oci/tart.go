package oci

import (
	"encoding/json"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// TartDisplay represents the display configuration in a Tart config.
type TartDisplay struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// TartConfig represents the configuration for a Tart (Cirrus Labs) VM image.
// This is parsed from layers[0] of a Tart OCI image manifest.
type TartConfig struct {
	// OS is the operating system (always "darwin" for macOS).
	OS string `json:"os"`

	// Arch is the CPU architecture (always "arm64" for Apple Silicon).
	Arch string `json:"arch"`

	// HardwareModel is the base64-encoded hardware model data.
	HardwareModel string `json:"hardwareModel"`

	// ECID is the base64-encoded machine identifier.
	ECID string `json:"ecid"`

	// CPUCount is the number of CPUs allocated to the VM.
	CPUCount int `json:"cpuCount"`

	// CPUCountMin is the minimum number of CPUs required.
	CPUCountMin int `json:"cpuCountMin"`

	// MemorySize is the amount of memory in bytes.
	MemorySize int64 `json:"memorySize"`

	// MemorySizeMin is the minimum memory in bytes.
	MemorySizeMin int64 `json:"memorySizeMin"`

	// MacAddress is the MAC address for the VM network interface.
	MacAddress string `json:"macAddress"`

	// Display contains the display configuration.
	Display TartDisplay `json:"display"`

	// DiskFormat is the disk format (usually "raw").
	DiskFormat string `json:"diskFormat"`

	// Version is the config format version.
	Version int `json:"version"`
}

// ToConfig converts a TartConfig to our internal Config format.
func (tc *TartConfig) ToConfig() Config {
	return Config{
		OS:                tc.OS,
		HardwareModelData: tc.HardwareModel,
		MachineIdData:     tc.ECID,
		Storage: []MediaType{
			MediaTypeAuxImage,  // NVRAM
			MediaTypeDiskImage, // Disk
		},
		SourceFormat: "tart",
	}
}

// ParseTartConfig parses a TartConfig from JSON bytes.
func ParseTartConfig(data []byte) (*TartConfig, error) {
	var config TartConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// IsTartManifest checks if a manifest is a Tart image by examining layers[0].
// IMPORTANT: Tart config is in layers[0], NOT manifest.Config!
func IsTartManifest(manifest ocispec.Manifest) bool {
	if len(manifest.Layers) == 0 {
		return false
	}
	return IsTartConfig(manifest.Layers[0].MediaType)
}

// CategorizeTartLayers separates a Tart manifest's layers into config, disk, and NVRAM.
// Returns:
//   - config: pointer to the config layer descriptor (layers[0]), or nil if not found
//   - diskLayers: slice of disk layer descriptors
//   - nvram: pointer to the NVRAM layer descriptor, or nil if not found
func CategorizeTartLayers(manifest ocispec.Manifest) (config *ocispec.Descriptor, diskLayers []ocispec.Descriptor, nvram *ocispec.Descriptor) {
	for i := range manifest.Layers {
		layer := &manifest.Layers[i]
		switch {
		case IsTartConfig(layer.MediaType):
			config = layer
		case IsTartDiskLayer(layer.MediaType):
			diskLayers = append(diskLayers, *layer)
		case layer.MediaType == string(TartNVRAMMediaType):
			nvram = layer
		}
	}
	return config, diskLayers, nvram
}

// ValidateTartLayers validates that all required Tart layers are present.
func ValidateTartLayers(config *ocispec.Descriptor, diskLayers []ocispec.Descriptor, nvram *ocispec.Descriptor) error {
	if config == nil {
		return fmt.Errorf("tart manifest missing config layer (expected in layers[0])")
	}
	if len(diskLayers) == 0 {
		return fmt.Errorf("tart manifest contains no disk layers")
	}
	if nvram == nil {
		return fmt.Errorf("tart manifest missing NVRAM layer")
	}
	return nil
}
