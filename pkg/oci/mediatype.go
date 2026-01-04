package oci

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

// MediaType represents a media type.
type MediaType string

const (
	// MediaTypeDiskImage specifies the media type for a disk image.
	MediaTypeDiskImage MediaType = "application/vnd.agoda.macosvz.disk.image.v1"

	// MediaTypeAuxImage specifies the media type for an auxiliary (nvram) image.
	MediaTypeAuxImage MediaType = "application/vnd.agoda.macosvz.aux.image.v1"

	// MediaTypeConfigV1 specifies the media type for a configuration.
	// Internal use only.
	MediaTypeConfigV1 MediaType = "application/vnd.agoda.macosvz.config.v1+json"

	// Tart (Cirrus Labs) media types
	// TartConfigMediaType specifies the media type for Tart VM configuration.
	TartConfigMediaType MediaType = "application/vnd.cirruslabs.tart.config.v1"

	// TartDiskMediaTypeV1 specifies the media type for Tart disk layers (v1).
	TartDiskMediaTypeV1 MediaType = "application/vnd.cirruslabs.tart.disk.v1"

	// TartDiskMediaTypeV2 specifies the media type for Tart disk layers (v2).
	TartDiskMediaTypeV2 MediaType = "application/vnd.cirruslabs.tart.disk.v2"

	// TartNVRAMMediaType specifies the media type for Tart NVRAM.
	TartNVRAMMediaType MediaType = "application/vnd.cirruslabs.tart.nvram.v1"
)

// mediaTypeToTitle maps media types to their titles.
var mediaTypeToTitle = map[MediaType]string{
	MediaTypeConfigV1:  "config.json",
	MediaTypeDiskImage: "disk.img",
	MediaTypeAuxImage:  "aux.img",
	// Tart media type titles
	TartConfigMediaType: "config.json",
	TartDiskMediaTypeV1: "disk.img",
	TartDiskMediaTypeV2: "disk.img",
	TartNVRAMMediaType:  "nvram.bin",
}

// Title returns the title of the media type.
func (mt MediaType) Title() string {
	return mediaTypeToTitle[mt]
}

// supportedMediaTypes contains the supported media types.
var supportedMediaTypes = sets.NewString(
	string(MediaTypeConfigV1),
	string(MediaTypeDiskImage),
	string(MediaTypeAuxImage),
	// Tart media types
	string(TartConfigMediaType),
	string(TartDiskMediaTypeV1),
	string(TartDiskMediaTypeV2),
	string(TartNVRAMMediaType),
)

// IsMediaTypeSupported checks if the media type is supported.
func IsMediaTypeSupported(mediaType string) bool {
	return supportedMediaTypes.Has(mediaType)
}

// IsTartMediaType checks if the media type is a Tart (Cirrus Labs) media type.
func IsTartMediaType(mediaType string) bool {
	return strings.HasPrefix(mediaType, "application/vnd.cirruslabs.tart")
}

// IsTartDiskLayer checks if the media type is a Tart disk layer.
func IsTartDiskLayer(mediaType string) bool {
	return mediaType == string(TartDiskMediaTypeV1) || mediaType == string(TartDiskMediaTypeV2)
}

// IsTartConfig checks if the media type is a Tart config.
func IsTartConfig(mediaType string) bool {
	return mediaType == string(TartConfigMediaType)
}
