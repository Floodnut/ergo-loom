package toolruntime

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

type ShellExecutor struct {
	Shell      string
	WorkingDir string
}

func (e ShellExecutor) Execute(ctx context.Context, request Request, emit func(Event)) (Result, error) {
	shell := e.Shell
	if shell == "" {
		shell = "/bin/zsh"
	}

	emit(Event{
		Type:         EventToolStart,
		ToolID:       request.ToolID,
		ToolName:     shell,
		InvocationID: request.InvocationID,
		Command:      request.Command,
		Status:       "running",
	})

	cmd := exec.CommandContext(ctx, shell, "-lc", request.Command)
	cmd.Dir = e.WorkingDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	status := "completed"
	if err != nil {
		status = "failed"
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if ctx.Err() == context.DeadlineExceeded {
		status = "timeout"
		exitCode = 124
	}

	result := Result{
		InvocationID: request.InvocationID,
		Status:       status,
		Stdout:       stdout.String(),
		Stderr:       stderr.String(),
		ExitCode:     exitCode,
	}
	emit(Event{
		Type:         EventToolResult,
		ToolID:       request.ToolID,
		ToolName:     shell,
		InvocationID: request.InvocationID,
		Command:      request.Command,
		Text:         stdout.String() + stderr.String(),
		Status:       status,
	})
	return result, err
}
