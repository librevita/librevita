package acme

import (
	"bytes"
	"context"
	"os"
	"os/exec"

	"librevita.org/pkg/errors"
)

// ExecProvider runs an external executable or script to handle DNS-01 challenges.
type ExecProvider struct {
	script string
}

// NewExecProvider creates an ExecProvider with the specified script path.
func NewExecProvider(script string) *ExecProvider {
	return &ExecProvider{script: script}
}

func (p *ExecProvider) run(ctx context.Context, action, domain, keyAuthRecord string) error {
	recordName := ChallengeRecordName(domain)
	cmd := exec.CommandContext(ctx, p.script, action, domain, keyAuthRecord) // #nosec G204 -- User-configured hook script path and ACME arguments.
	cmd.Env = append(os.Environ(),
		"LIBREVITA_ACME_ACTION="+action,
		"LIBREVITA_ACME_DOMAIN="+domain,
		"LIBREVITA_ACME_RECORD_NAME="+recordName,
		"LIBREVITA_ACME_VALUE="+keyAuthRecord,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "acme exec hook %s failed: %s", action, stderr.String())
	}
	return nil
}

func (p *ExecProvider) Present(ctx context.Context, domain, keyAuthRecord string) error {
	return p.run(ctx, "present", domain, keyAuthRecord)
}

func (p *ExecProvider) CleanUp(ctx context.Context, domain, keyAuthRecord string) error {
	return p.run(ctx, "cleanup", domain, keyAuthRecord)
}
