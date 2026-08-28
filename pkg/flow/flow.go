package flow

import (
	"github.com/cockroachdb/errors"
)

// StepFunc is a unit of execution in a pipeline.
type StepFunc func() error

// RollbackFunc is a compensation action executed if a subsequent step fails.
type RollbackFunc func() error

type stepEntry struct {
	name     string
	rollback RollbackFunc
}

// Pipeline orchestrates sequential execution of steps with optional rollback compensation.
type Pipeline struct {
	rollbacks []stepEntry
	err       error
}

// New creates a new Pipeline instance.
func New() *Pipeline {
	return &Pipeline{}
}

// Step registers and immediately executes a step with a descriptive name.
// If an earlier step in the pipeline failed, subsequent steps are skipped.
func (p *Pipeline) Step(name string, fn StepFunc) *Pipeline {
	if p.err != nil || fn == nil {
		return p
	}
	if err := fn(); err != nil {
		p.fail(name, err)
	}
	return p
}

// StepIf conditionally registers and executes a step only if condition is true.
func (p *Pipeline) StepIf(condition bool, name string, fn StepFunc) *Pipeline {
	if !condition {
		return p
	}
	return p.Step(name, fn)
}

// StepWithRollback executes a step and records a rollback compensation function
// that is automatically executed in reverse (LIFO) order if any subsequent step fails.
func (p *Pipeline) StepWithRollback(name string, fn StepFunc, rollback RollbackFunc) *Pipeline {
	if p.err != nil || fn == nil {
		return p
	}
	if err := fn(); err != nil {
		p.fail(name, err)
		return p
	}
	if rollback != nil {
		p.rollbacks = append(p.rollbacks, stepEntry{name: name, rollback: rollback})
	}
	return p
}

func (p *Pipeline) fail(name string, err error) {
	if name != "" {
		p.err = errors.Wrapf(err, "flow: %s", name)
	} else {
		p.err = err
	}

	// Execute recorded compensations in reverse order
	for i := len(p.rollbacks) - 1; i >= 0; i-- {
		entry := p.rollbacks[i]
		if rerr := entry.rollback(); rerr != nil {
			p.err = errors.Join(p.err, errors.Wrapf(rerr, "flow: rollback %s", entry.name))
		}
	}
	p.rollbacks = nil
}

// Err returns the first error encountered during pipeline execution,
// combined with any rollback compensation errors, or nil if all steps succeeded.
func (p *Pipeline) Err() error {
	return p.err
}

// Exec executes functions sequentially and stops at the first error.
func Exec(steps ...StepFunc) error {
	for _, fn := range steps {
		if fn == nil {
			continue
		}
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

// All executes all provided functions regardless of failures,
// collecting and returning all encountered errors combined via errors.Join.
func All(steps ...StepFunc) error {
	var errs []error
	for _, fn := range steps {
		if fn == nil {
			continue
		}
		if err := fn(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
