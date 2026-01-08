package config

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/Code-Hex/vz/v3"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"

	"github.com/agoda-com/macOS-vz-kubelet/internal/utils"
)

// MacPlatformConfigurationOptions holds the options for creating a new PlatformConfiguration.
type MacPlatformConfigurationOptions struct {
	BlockStoragePath      string
	AuxiliaryStoragePath  string
	HardwareModelData     string
	MachineIdentifierData string
	SourceFormat          string // "oras" or "tart" - indicates the original image format
}

// PlatformConfiguration holds the configuration for the platform, including storage paths and overlay usage.
type PlatformConfiguration struct {
	BlockStoragePath     string
	AuxiliaryStoragePath string
	IsOverlay            bool

	*vz.MacPlatformConfiguration
}

// NewPlatformConfiguration creates a new PlatformConfiguration.
func NewPlatformConfiguration(ctx context.Context, opts MacPlatformConfigurationOptions, useOverlay bool, uid string) (p *PlatformConfiguration, err error) {
	ctx, span := trace.StartSpan(ctx, "platform.NewPlatformConfiguration")
	defer func() {
		span.SetStatus(err)
		span.End()
	}()

	blockStoragePath := opts.BlockStoragePath
	auxiliaryStoragePath := opts.AuxiliaryStoragePath
	if useOverlay {
		fc := utils.NewFileCloner()

		blockStoragePath, err = fc.Clonefile(blockStoragePath, uid)
		if err != nil {
			return nil, err
		}
		ctx = span.WithField(ctx, "blockStoragePath", blockStoragePath)

		auxiliaryStoragePath, err = fc.Clonefile(auxiliaryStoragePath, uid)
		if err != nil {
			return nil, err
		}
		ctx = span.WithField(ctx, "auxiliaryStoragePath", auxiliaryStoragePath)
	}

	_ = span.WithFields(ctx, log.Fields{
		"blockStoragePath":     blockStoragePath,
		"auxiliaryStoragePath": auxiliaryStoragePath,
		"sourceFormat":         opts.SourceFormat,
	})

	decodedHardwareModelData, err := base64.StdEncoding.DecodeString(opts.HardwareModelData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hardware model data: %w", err)
	}

	hardwareModel, err := vz.NewMacHardwareModelWithData(decodedHardwareModelData)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new hardware model: %w", err)
	}

	// Load auxiliary storage (NVRAM)
	// Both Tart and ORAS images provide NVRAM that should be used as-is
	log.G(ctx).Infof("Loading existing auxiliary storage from %s", auxiliaryStoragePath)
	auxiliaryStorage, err := vz.NewMacAuxiliaryStorage(auxiliaryStoragePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create auxiliary storage: %w", err)
	}

	decodedMachineIdentifierData, err := base64.StdEncoding.DecodeString(opts.MachineIdentifierData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode machine identifier data: %w", err)
	}

	machineIdentifier, err := vz.NewMacMachineIdentifierWithData(decodedMachineIdentifierData)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new machine identifier: %w", err)
	}

	c, err := vz.NewMacPlatformConfiguration(
		vz.WithMacAuxiliaryStorage(auxiliaryStorage),
		vz.WithMacHardwareModel(hardwareModel),
		vz.WithMacMachineIdentifier(machineIdentifier),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new mac platform configuration: %w", err)
	}

	return &PlatformConfiguration{
		BlockStoragePath:         blockStoragePath,
		AuxiliaryStoragePath:     auxiliaryStoragePath,
		IsOverlay:                useOverlay,
		MacPlatformConfiguration: c,
	}, nil
}
