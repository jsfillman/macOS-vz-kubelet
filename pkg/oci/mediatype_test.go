package oci

import (
	"testing"
)

func TestTartMediaTypeConstants(t *testing.T) {
	// Verify Tart media type constants exist and have correct values
	tests := []struct {
		name string
		got  MediaType
		want string
	}{
		{"TartConfigMediaType", TartConfigMediaType, "application/vnd.cirruslabs.tart.config.v1"},
		{"TartDiskMediaTypeV1", TartDiskMediaTypeV1, "application/vnd.cirruslabs.tart.disk.v1"},
		{"TartDiskMediaTypeV2", TartDiskMediaTypeV2, "application/vnd.cirruslabs.tart.disk.v2"},
		{"TartNVRAMMediaType", TartNVRAMMediaType, "application/vnd.cirruslabs.tart.nvram.v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestIsTartMediaType(t *testing.T) {
	tests := []struct {
		mediaType string
		want      bool
	}{
		{"application/vnd.cirruslabs.tart.config.v1", true},
		{"application/vnd.cirruslabs.tart.disk.v1", true},
		{"application/vnd.cirruslabs.tart.disk.v2", true},
		{"application/vnd.cirruslabs.tart.nvram.v1", true},
		{"application/vnd.cirruslabs.tart.future.v3", true}, // Any tart type
		{"application/vnd.agoda.macosvz.disk.image.v1", false},
		{"application/vnd.oci.image.config.v1+json", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			if got := IsTartMediaType(tt.mediaType); got != tt.want {
				t.Errorf("IsTartMediaType(%q) = %v, want %v", tt.mediaType, got, tt.want)
			}
		})
	}
}

func TestIsTartDiskLayer(t *testing.T) {
	tests := []struct {
		mediaType string
		want      bool
	}{
		{"application/vnd.cirruslabs.tart.disk.v1", true},
		{"application/vnd.cirruslabs.tart.disk.v2", true},
		{"application/vnd.cirruslabs.tart.config.v1", false},
		{"application/vnd.cirruslabs.tart.nvram.v1", false},
		{"application/vnd.agoda.macosvz.disk.image.v1", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			if got := IsTartDiskLayer(tt.mediaType); got != tt.want {
				t.Errorf("IsTartDiskLayer(%q) = %v, want %v", tt.mediaType, got, tt.want)
			}
		})
	}
}

func TestIsTartConfig(t *testing.T) {
	tests := []struct {
		mediaType string
		want      bool
	}{
		{"application/vnd.cirruslabs.tart.config.v1", true},
		{"application/vnd.cirruslabs.tart.disk.v1", false},
		{"application/vnd.cirruslabs.tart.disk.v2", false},
		{"application/vnd.cirruslabs.tart.nvram.v1", false},
		{"application/vnd.agoda.macosvz.config.v1+json", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			if got := IsTartConfig(tt.mediaType); got != tt.want {
				t.Errorf("IsTartConfig(%q) = %v, want %v", tt.mediaType, got, tt.want)
			}
		})
	}
}

func TestTartMediaTypesAreSupported(t *testing.T) {
	// Tart media types should be recognized as supported
	tartTypes := []string{
		"application/vnd.cirruslabs.tart.config.v1",
		"application/vnd.cirruslabs.tart.disk.v1",
		"application/vnd.cirruslabs.tart.disk.v2",
		"application/vnd.cirruslabs.tart.nvram.v1",
	}

	for _, mt := range tartTypes {
		t.Run(mt, func(t *testing.T) {
			if !IsMediaTypeSupported(mt) {
				t.Errorf("IsMediaTypeSupported(%q) = false, want true", mt)
			}
		})
	}
}

func TestTartMediaTypeTitle(t *testing.T) {
	tests := []struct {
		mediaType MediaType
		want      string
	}{
		{TartConfigMediaType, "config.json"},
		{TartDiskMediaTypeV1, "disk.img"},
		{TartDiskMediaTypeV2, "disk.img"},
		{TartNVRAMMediaType, "nvram.bin"},
	}

	for _, tt := range tests {
		t.Run(string(tt.mediaType), func(t *testing.T) {
			if got := tt.mediaType.Title(); got != tt.want {
				t.Errorf("%s.Title() = %q, want %q", tt.mediaType, got, tt.want)
			}
		})
	}
}
