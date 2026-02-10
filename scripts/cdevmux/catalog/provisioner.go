package catalog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

// No local constants needed - use names.go

// Provisioner runs external provisioner daemons.
type Provisioner struct {
	Count int    // Number of provisioner daemons to run.
	Org   string // Organization to attach to (empty for default).

	mu      sync.Mutex
	cmds    []*exec.Cmd
	doneChans []chan struct{}
}

func NewProvisioner() *Provisioner {
	return &Provisioner{
		Count: 1,
	}
}

func (p *Provisioner) Name() string {
	if p.Org != "" {
		return ProvisionerName + "/" + p.Org
	}
	return ProvisionerName
}

func (p *Provisioner) DependsOn() []string {
	return []string{CoderdName}
}

func (p *Provisioner) EnablementFlag() string {
	return "--external-provisioner"
}

func (p *Provisioner) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := 0; i < p.Count; i++ {
		args := []string{"provisionerd", "start",
			"--name", fmt.Sprintf("cdev-provisioner-%d", i),
		}
		if p.Org != "" {
			args = append(args, "--org", p.Org)
		}

		cmd := exec.CommandContext(ctx, "./.coderv2/coder", args...)
		cmd.Env = os.Environ()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			// Stop any already-started provisioners.
			p.stopAll(ctx)
			return fmt.Errorf("failed to start provisioner %d: %w", i, err)
		}

		done := make(chan struct{})
		go func(c *exec.Cmd) {
			_ = c.Wait()
			close(done)
		}(cmd)

		p.cmds = append(p.cmds, cmd)
		p.doneChans = append(p.doneChans, done)
	}

	return nil
}

func (p *Provisioner) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopAll(ctx)
}

func (p *Provisioner) stopAll(ctx context.Context) error {
	var firstErr error
	for i, cmd := range p.cmds {
		if cmd == nil || cmd.Process == nil {
			continue
		}

		if err := cmd.Process.Signal(os.Interrupt); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to send interrupt to provisioner %d: %w", i, err)
		}

		select {
		case <-p.doneChans[i]:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		case <-ctx.Done():
			_ = cmd.Process.Kill()
		}
	}

	p.cmds = nil
	p.doneChans = nil
	return firstErr
}
