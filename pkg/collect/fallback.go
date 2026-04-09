package collect

import (
	"context"
	"fmt"
	"os/exec"

	"gorancid/pkg/config"
	"gorancid/pkg/devicetype"
)

// FallbackCollector delegates per-device collection to the original Perl rancid script.
// Used for device types that do not yet have a Go parser registered.
type FallbackCollector struct {
	Device    config.Device
	Spec      devicetype.DeviceSpec
	Creds     config.Credentials
	OutDir    string
	RancidBin string // path to original rancid binary; defaults to "rancid"
}

// Run executes the original rancid script for this device.
// A non-zero exit from rancid is treated as StatusFailed — not a hard error —
// so the caller can aggregate results across a group.
func (c *FallbackCollector) Run(ctx context.Context) (Result, error) {
	bin := c.RancidBin
	if bin == "" {
		bin = "rancid"
	}

	cmd := exec.CommandContext(ctx, bin, "-t", c.Device.Type, c.Device.Hostname)
	cmd.Dir = c.OutDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{
			Hostname: c.Device.Hostname,
			Status:   StatusFailed,
			Error:    fmt.Errorf("%s: %w\n%s", bin, err, out),
		}, nil
	}
	return Result{
		Hostname: c.Device.Hostname,
		Status:   StatusSuccess,
	}, nil
}