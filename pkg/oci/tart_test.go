package oci

import (
	"encoding/json"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// createTestManifest creates a test OCI manifest with the given layer media types.
func createTestManifest(layers []struct{ mediaType string }) ocispec.Manifest {
	manifest := ocispec.Manifest{
		Layers: make([]ocispec.Descriptor, len(layers)),
	}
	for i, l := range layers {
		manifest.Layers[i] = ocispec.Descriptor{
			MediaType: l.mediaType,
		}
	}
	return manifest
}

// Sample TartConfig JSON from ghcr.io/cirruslabs/macos-sequoia-base:latest layers[0]
const sampleTartConfigJSON = `{
  "arch": "arm64",
  "cpuCount": 4,
  "cpuCountMin": 2,
  "diskFormat": "raw",
  "display": {
    "height": 768,
    "width": 1024
  },
  "ecid": "YnBsaXN0MDDRAQJURUNJRBMYcKtvqAvuVAgLEAAAAAAAAAEBAAAAAAAAAAMAAAAAAAAAAAAAAAAAAAAZ",
  "hardwareModel": "YnBsaXN0MDDTAQIDBAQFXxAZRGF0YVJlcHJlc2VudGF0aW9uVmVyc2lvbl8QD1BsYXRmb3JtVmVyc2lvbl8QEk1pbmltdW1TdXBwb3J0ZWRPUxACowYHBxANEAAIDys9UlRYWgAAAAAAAAEBAAAAAAAAAAgAAAAAAAAAAAAAAAAAAABc",
  "macAddress": "06:e8:a3:b0:23:b4",
  "memorySize": 8589934592,
  "memorySizeMin": 4294967296,
  "os": "darwin",
  "version": 1
}`

func TestTartConfig_UnmarshalJSON(t *testing.T) {
	var config TartConfig
	err := json.Unmarshal([]byte(sampleTartConfigJSON), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal TartConfig: %v", err)
	}

	// Verify required fields
	if config.OS != "darwin" {
		t.Errorf("OS = %q, want %q", config.OS, "darwin")
	}
	if config.Arch != "arm64" {
		t.Errorf("Arch = %q, want %q", config.Arch, "arm64")
	}
	if config.HardwareModel == "" {
		t.Error("HardwareModel should not be empty")
	}
	if config.ECID == "" {
		t.Error("ECID should not be empty")
	}
	if config.CPUCount != 4 {
		t.Errorf("CPUCount = %d, want %d", config.CPUCount, 4)
	}
	if config.MemorySize != 8589934592 {
		t.Errorf("MemorySize = %d, want %d", config.MemorySize, 8589934592)
	}
}

func TestTartConfig_ToConfig(t *testing.T) {
	var tartConfig TartConfig
	err := json.Unmarshal([]byte(sampleTartConfigJSON), &tartConfig)
	if err != nil {
		t.Fatalf("Failed to unmarshal TartConfig: %v", err)
	}

	config := tartConfig.ToConfig()

	// Verify conversion
	if config.OS != "darwin" {
		t.Errorf("Config.OS = %q, want %q", config.OS, "darwin")
	}
	if config.HardwareModelData != tartConfig.HardwareModel {
		t.Errorf("Config.HardwareModelData = %q, want %q", config.HardwareModelData, tartConfig.HardwareModel)
	}
	if config.MachineIdData != tartConfig.ECID {
		t.Errorf("Config.MachineIdData = %q, want %q", config.MachineIdData, tartConfig.ECID)
	}
	if config.SourceFormat != "tart" {
		t.Errorf("Config.SourceFormat = %q, want %q", config.SourceFormat, "tart")
	}
}

func TestTartConfig_Display(t *testing.T) {
	var config TartConfig
	err := json.Unmarshal([]byte(sampleTartConfigJSON), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal TartConfig: %v", err)
	}

	if config.Display.Width != 1024 {
		t.Errorf("Display.Width = %d, want %d", config.Display.Width, 1024)
	}
	if config.Display.Height != 768 {
		t.Errorf("Display.Height = %d, want %d", config.Display.Height, 768)
	}
}

func TestTartConfig_MinValues(t *testing.T) {
	var config TartConfig
	err := json.Unmarshal([]byte(sampleTartConfigJSON), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal TartConfig: %v", err)
	}

	if config.CPUCountMin != 2 {
		t.Errorf("CPUCountMin = %d, want %d", config.CPUCountMin, 2)
	}
	if config.MemorySizeMin != 4294967296 {
		t.Errorf("MemorySizeMin = %d, want %d", config.MemorySizeMin, 4294967296)
	}
}

func TestTartConfig_EmptyJSON(t *testing.T) {
	var config TartConfig
	err := json.Unmarshal([]byte(`{}`), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal empty TartConfig: %v", err)
	}

	// All fields should be zero/empty values
	if config.OS != "" {
		t.Errorf("OS should be empty, got %q", config.OS)
	}
	if config.HardwareModel != "" {
		t.Errorf("HardwareModel should be empty, got %q", config.HardwareModel)
	}
}

func TestParseTartConfig(t *testing.T) {
	config, err := ParseTartConfig([]byte(sampleTartConfigJSON))
	if err != nil {
		t.Fatalf("ParseTartConfig failed: %v", err)
	}

	if config.OS != "darwin" {
		t.Errorf("OS = %q, want %q", config.OS, "darwin")
	}
	if config.HardwareModel == "" {
		t.Error("HardwareModel should not be empty")
	}
}

func TestParseTartConfig_InvalidJSON(t *testing.T) {
	_, err := ParseTartConfig([]byte(`{invalid json`))
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestIsTartManifest(t *testing.T) {
	tests := []struct {
		name   string
		layers []struct {
			mediaType string
		}
		want bool
	}{
		{
			name: "Tart manifest with config in layers[0]",
			layers: []struct {
				mediaType string
			}{
				{mediaType: "application/vnd.cirruslabs.tart.config.v1"},
				{mediaType: "application/vnd.cirruslabs.tart.disk.v2"},
				{mediaType: "application/vnd.cirruslabs.tart.disk.v2"},
				{mediaType: "application/vnd.cirruslabs.tart.nvram.v1"},
			},
			want: true,
		},
		{
			name: "Non-Tart manifest (ORAS format)",
			layers: []struct {
				mediaType string
			}{
				{mediaType: "application/vnd.agoda.macosvz.disk.image.v1"},
				{mediaType: "application/vnd.agoda.macosvz.aux.image.v1"},
			},
			want: false,
		},
		{
			name:   "Empty manifest",
			layers: nil,
			want:   false,
		},
		{
			name: "Tart disk in layers[0] (not a valid Tart manifest)",
			layers: []struct {
				mediaType string
			}{
				{mediaType: "application/vnd.cirruslabs.tart.disk.v2"},
				{mediaType: "application/vnd.cirruslabs.tart.nvram.v1"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := createTestManifest(tt.layers)
			got := IsTartManifest(manifest)
			if got != tt.want {
				t.Errorf("IsTartManifest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCategorizeTartLayers(t *testing.T) {
	layers := []struct {
		mediaType string
	}{
		{mediaType: "application/vnd.cirruslabs.tart.config.v1"},
		{mediaType: "application/vnd.cirruslabs.tart.disk.v2"},
		{mediaType: "application/vnd.cirruslabs.tart.disk.v2"},
		{mediaType: "application/vnd.cirruslabs.tart.disk.v2"},
		{mediaType: "application/vnd.cirruslabs.tart.nvram.v1"},
	}

	manifest := createTestManifest(layers)
	config, diskLayers, nvram := CategorizeTartLayers(manifest)

	if config == nil {
		t.Error("Expected config layer, got nil")
	} else if config.MediaType != "application/vnd.cirruslabs.tart.config.v1" {
		t.Errorf("Config MediaType = %q, want %q", config.MediaType, "application/vnd.cirruslabs.tart.config.v1")
	}

	if len(diskLayers) != 3 {
		t.Errorf("Expected 3 disk layers, got %d", len(diskLayers))
	}

	if nvram == nil {
		t.Error("Expected nvram layer, got nil")
	} else if nvram.MediaType != "application/vnd.cirruslabs.tart.nvram.v1" {
		t.Errorf("NVRAM MediaType = %q, want %q", nvram.MediaType, "application/vnd.cirruslabs.tart.nvram.v1")
	}
}

func TestCategorizeTartLayers_MissingConfig(t *testing.T) {
	layers := []struct {
		mediaType string
	}{
		{mediaType: "application/vnd.cirruslabs.tart.disk.v2"},
		{mediaType: "application/vnd.cirruslabs.tart.nvram.v1"},
	}

	manifest := createTestManifest(layers)
	config, _, _ := CategorizeTartLayers(manifest)

	if config != nil {
		t.Error("Expected nil config for manifest without config layer")
	}
}

func TestCategorizeTartLayers_MissingNVRAM(t *testing.T) {
	layers := []struct {
		mediaType string
	}{
		{mediaType: "application/vnd.cirruslabs.tart.config.v1"},
		{mediaType: "application/vnd.cirruslabs.tart.disk.v2"},
	}

	manifest := createTestManifest(layers)
	_, _, nvram := CategorizeTartLayers(manifest)

	if nvram != nil {
		t.Error("Expected nil nvram for manifest without nvram layer")
	}
}

func TestValidateTartLayers(t *testing.T) {
	tests := []struct {
		name        string
		config      *ocispec.Descriptor
		diskLayers  []ocispec.Descriptor
		nvram       *ocispec.Descriptor
		expectError bool
	}{
		{
			name:        "Valid layers",
			config:      &ocispec.Descriptor{MediaType: "application/vnd.cirruslabs.tart.config.v1"},
			diskLayers:  []ocispec.Descriptor{{MediaType: "application/vnd.cirruslabs.tart.disk.v2"}},
			nvram:       &ocispec.Descriptor{MediaType: "application/vnd.cirruslabs.tart.nvram.v1"},
			expectError: false,
		},
		{
			name:        "Missing config",
			config:      nil,
			diskLayers:  []ocispec.Descriptor{{MediaType: "application/vnd.cirruslabs.tart.disk.v2"}},
			nvram:       &ocispec.Descriptor{MediaType: "application/vnd.cirruslabs.tart.nvram.v1"},
			expectError: true,
		},
		{
			name:        "Missing disk layers",
			config:      &ocispec.Descriptor{MediaType: "application/vnd.cirruslabs.tart.config.v1"},
			diskLayers:  nil,
			nvram:       &ocispec.Descriptor{MediaType: "application/vnd.cirruslabs.tart.nvram.v1"},
			expectError: true,
		},
		{
			name:        "Missing nvram",
			config:      &ocispec.Descriptor{MediaType: "application/vnd.cirruslabs.tart.config.v1"},
			diskLayers:  []ocispec.Descriptor{{MediaType: "application/vnd.cirruslabs.tart.disk.v2"}},
			nvram:       nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTartLayers(tt.config, tt.diskLayers, tt.nvram)
			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
