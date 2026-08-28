package flow_test

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/flow"
)

func TestPipeline_Success(t *testing.T) {
	var order []string

	err := flow.New().
		Step("step 1", func() error {
			order = append(order, "step 1")
			return nil
		}).
		Step("step 2", func() error {
			order = append(order, "step 2")
			return nil
		}).
		Err()

	require.NoError(t, err)
	assert.Equal(t, []string{"step 1", "step 2"}, order)
}

func TestPipeline_SkipsSubsequentOnFailure(t *testing.T) {
	var order []string
	sentinel := errors.New("db failure")

	err := flow.New().
		Step("step 1", func() error {
			order = append(order, "step 1")
			return nil
		}).
		Step("failing step", func() error {
			order = append(order, "failing step")
			return sentinel
		}).
		Step("step 3", func() error {
			order = append(order, "step 3")
			return nil
		}).
		Err()

	require.Error(t, err)
	assert.True(t, errors.Is(err, sentinel))
	assert.Contains(t, err.Error(), "flow: failing step")
	assert.Equal(t, []string{"step 1", "failing step"}, order)
}

func TestPipeline_StepIf(t *testing.T) {
	var executed []string

	err := flow.New().
		StepIf(false, "skipped", func() error {
			executed = append(executed, "skipped")
			return nil
		}).
		StepIf(true, "executed", func() error {
			executed = append(executed, "executed")
			return nil
		}).
		Err()

	require.NoError(t, err)
	assert.Equal(t, []string{"executed"}, executed)
}

func TestPipeline_StepWithRollback_LIFO(t *testing.T) {
	var actions []string
	sentinel := errors.New("step 3 failed")

	err := flow.New().
		StepWithRollback("step 1", func() error {
			actions = append(actions, "run 1")
			return nil
		}, func() error {
			actions = append(actions, "rollback 1")
			return nil
		}).
		StepWithRollback("step 2", func() error {
			actions = append(actions, "run 2")
			return nil
		}, func() error {
			actions = append(actions, "rollback 2")
			return nil
		}).
		Step("step 3", func() error {
			actions = append(actions, "run 3")
			return sentinel
		}).
		Err()

	require.Error(t, err)
	assert.True(t, errors.Is(err, sentinel))
	// Rollbacks must run in LIFO order: 2 then 1
	assert.Equal(t, []string{"run 1", "run 2", "run 3", "rollback 2", "rollback 1"}, actions)
}

func TestPipeline_StepWithRollback_CombinesErrors(t *testing.T) {
	stepErr := errors.New("step 2 failed")
	rollbackErr := errors.New("rollback 1 failed")

	err := flow.New().
		StepWithRollback("step 1", func() error {
			return nil
		}, func() error {
			return rollbackErr
		}).
		Step("step 2", func() error {
			return stepErr
		}).
		Err()

	require.Error(t, err)
	assert.True(t, errors.Is(err, stepErr))
	assert.True(t, errors.Is(err, rollbackErr))
}

func TestExec(t *testing.T) {
	count := 0
	sentinel := errors.New("boom")

	err := flow.Exec(
		func() error { count++; return nil },
		func() error { count++; return sentinel },
		func() error { count++; return nil },
	)

	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 2, count)
}

func TestAll(t *testing.T) {
	err1 := errors.New("err 1")
	err2 := errors.New("err 2")
	ran := 0

	err := flow.All(
		func() error { ran++; return err1 },
		func() error { ran++; return nil },
		func() error { ran++; return err2 },
	)

	require.Error(t, err)
	assert.True(t, errors.Is(err, err1))
	assert.True(t, errors.Is(err, err2))
	assert.Equal(t, 3, ran)
}
