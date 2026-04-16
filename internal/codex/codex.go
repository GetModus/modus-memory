package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Event represents a single JSONL event from the Codex CLI.
type Event struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id,omitempty"`
	Message  string `json:"message,omitempty"`

	// For item.completed
	Item *Item `json:"item,omitempty"`

	// For turn.completed
	Usage *Usage `json:"usage,omitempty"`

	// For errors
	Error *EventError `json:"error,omitempty"`
}

// Item is a completed item (message, tool call, error).
type Item struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "agent_message", "tool_call", "error"
	Text string `json:"text,omitempty"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

// EventError holds error details from a failed turn.
type EventError struct {
	Message string `json:"message"`
}

// RunOptions configures a Codex CLI invocation.
type RunOptions struct {
	Prompt    string
	Model     string // e.g. "gpt-5.4", or empty for default
	WorkDir   string
	Ephemeral bool // don't persist session files
}

// Result is the final output of a Codex CLI run.
type Result struct {
	Text     string
	IsError  bool
	ThreadID string
	Usage    *Usage
	Events   []Event
}

// Run executes the Codex CLI and returns the result.
func Run(ctx context.Context, opts RunOptions) (*Result, error) {
	args := []string{"exec", "--json"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	if opts.Ephemeral {
		args = append(args, "--ephemeral")
	}

	cmd := exec.CommandContext(ctx, "codex", args...)
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	cmd.Stdin = strings.NewReader(opts.Prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}

	var stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stderrBuf, stderr)
		close(stderrDone)
	}()

	var events []Event
	var result Result

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		events = append(events, ev)

		switch ev.Type {
		case "thread.started":
			result.ThreadID = ev.ThreadID
		case "item.completed":
			if ev.Item != nil {
				switch ev.Item.Type {
				case "agent_message":
					if result.Text != "" {
						result.Text += "\n"
					}
					result.Text += ev.Item.Text
				case "error":
					result.IsError = true
					result.Text = ev.Item.Text
					if result.Text == "" {
						result.Text = ev.Item.ID
					}
				}
			}
		case "turn.completed":
			if ev.Usage != nil {
				result.Usage = ev.Usage
			}
		case "turn.failed":
			result.IsError = true
			if ev.Error != nil {
				result.Text = ev.Error.Message
			}
		case "error":
			result.IsError = true
			if ev.Message != "" {
				result.Text = ev.Message
			}
		}
	}

	waitErr := cmd.Wait()
	<-stderrDone
	if waitErr != nil {
		stderrText := strings.TrimSpace(stderrBuf.String())
		if result.Text == "" {
			if stderrText != "" {
				return nil, fmt.Errorf("codex exited: %w: %s", waitErr, stderrText)
			}
			return nil, fmt.Errorf("codex exited: %w", waitErr)
		}
		if stderrText != "" {
			result.Text = strings.TrimSpace(result.Text + "\n" + stderrText)
		}
	}

	result.Events = events
	return &result, nil
}

// StreamEvent is sent on the channel during RunStream.
type StreamEvent struct {
	Event Event
	Text  string
	Done  bool
	Err   error
}

// RunStream executes the Codex CLI and streams events.
func RunStream(ctx context.Context, opts RunOptions) <-chan StreamEvent {
	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		args := []string{"exec", "--json"}
		if opts.Model != "" {
			args = append(args, "--model", opts.Model)
		}
		if opts.Ephemeral {
			args = append(args, "--ephemeral")
		}

		cmd := exec.CommandContext(ctx, "codex", args...)
		if opts.WorkDir != "" {
			cmd.Dir = opts.WorkDir
		}
		cmd.Stdin = strings.NewReader(opts.Prompt)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			ch <- StreamEvent{Err: fmt.Errorf("stdout pipe: %w", err)}
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			ch <- StreamEvent{Err: fmt.Errorf("stderr pipe: %w", err)}
			return
		}

		if err := cmd.Start(); err != nil {
			ch <- StreamEvent{Err: fmt.Errorf("start codex: %w", err)}
			return
		}

		var stderrBuf bytes.Buffer
		stderrDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(&stderrBuf, stderr)
			close(stderrDone)
		}()

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var ev Event
			if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
				continue
			}

			se := StreamEvent{Event: ev}

			switch ev.Type {
			case "item.completed":
				if ev.Item != nil && ev.Item.Type == "agent_message" {
					se.Text = ev.Item.Text
				}
			case "turn.completed":
				se.Done = true
			case "turn.failed", "error":
				se.Done = true
				if ev.Error != nil {
					se.Err = fmt.Errorf("codex: %s", ev.Error.Message)
				} else if ev.Message != "" {
					se.Err = fmt.Errorf("codex: %s", ev.Message)
				}
			}

			ch <- se
		}

		waitErr := cmd.Wait()
		<-stderrDone
		if waitErr != nil {
			stderrText := strings.TrimSpace(stderrBuf.String())
			if stderrText != "" {
				ch <- StreamEvent{Err: fmt.Errorf("codex exited: %w: %s", waitErr, stderrText), Done: true}
				return
			}
			ch <- StreamEvent{Err: fmt.Errorf("codex exited: %w", waitErr), Done: true}
		}
	}()

	return ch
}
