package runtime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alibaba/opensandbox/execd/pkg/jupyter/execute"
)

func TestDispatchExecutionResultHooks_ErrorSkipsComplete(t *testing.T) {
	var (
		errorCalls    int
		completeCalls int
	)

	req := &ExecuteCodeRequest{
		Hooks: ExecuteResultHook{
			OnExecuteError: func(_ *execute.ErrorOutput) {
				errorCalls++
			},
			OnExecuteComplete: func(_ time.Duration) {
				completeCalls++
			},
		},
	}

	dispatchExecutionResultHooks(req, &execute.ExecutionResult{
		ExecutionTime: 35 * time.Millisecond,
		Error: &execute.ErrorOutput{
			EName:  "RuntimeError",
			EValue: "boom",
		},
	})

	require.Equal(t, 1, errorCalls)
	require.Equal(t, 0, completeCalls)
}

func TestDispatchExecutionResultHooks_SuccessEmitsComplete(t *testing.T) {
	var (
		errorCalls    int
		completeCalls int
	)

	req := &ExecuteCodeRequest{
		Hooks: ExecuteResultHook{
			OnExecuteError: func(_ *execute.ErrorOutput) {
				errorCalls++
			},
			OnExecuteComplete: func(_ time.Duration) {
				completeCalls++
			},
		},
	}

	dispatchExecutionResultHooks(req, &execute.ExecutionResult{
		ExecutionTime: 50 * time.Millisecond,
	})

	require.Equal(t, 0, errorCalls)
	require.Equal(t, 1, completeCalls)
}
