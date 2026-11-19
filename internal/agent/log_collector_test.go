package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type stubLogCmdRunner struct {
	calls [][]string
	out   []byte
	err   error
}

func (s *stubLogCmdRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	s.calls = append(s.calls, call)
	return s.out, s.err
}

func TestJournalctlLogCollector_WhitelistRejectsInvalidServices(t *testing.T) {
	invalid := []string{
		"",
		"sshd",
		"nginx",
		"xray; rm -rf /",
		"xray && id",
		"xray\n",
		" xray",
		"xray-agent ",
		"../xray",
	}

	for _, svc := range invalid {
		runner := &stubLogCmdRunner{out: []byte("should not run")}
		collector := NewJournalctlLogCollector(runner)
		lines, err := collector.Collect(context.Background(), svc, 50)
		if err == nil {
			t.Fatalf("expected error for invalid service %q, got lines=%v", svc, lines)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("expected command runner not to be called for invalid service %q, got %v", svc, runner.calls)
		}
	}
}

func TestJournalctlLogCollector_LinesClamping(t *testing.T) {
	cases := []struct {
		name     string
		input    int
		expected string
	}{
		{name: "zero defaults to 100", input: 0, expected: "100"},
		{name: "negative defaults to 100", input: -25, expected: "100"},
		{name: "min 1 preserved", input: 1, expected: "1"},
		{name: "normal 250 preserved", input: 250, expected: "250"},
		{name: "max 1000 preserved", input: 1000, expected: "1000"},
		{name: "over 1000 clamped to 1000", input: 5000, expected: "1000"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &stubLogCmdRunner{out: []byte("line1\n")}
			collector := NewJournalctlLogCollector(runner)
			if _, err := collector.Collect(context.Background(), "xray-agent", tc.input); err != nil {
				t.Fatalf("Collect failed: %v", err)
			}
			if len(runner.calls) != 1 {
				t.Fatalf("expected 1 command call, got %d", len(runner.calls))
			}
			want := []string{"journalctl", "-u", "xray-agent", "-n", tc.expected, "--no-pager"}
			if !reflect.DeepEqual(runner.calls[0], want) {
				t.Fatalf("command mismatch: got %v, want %v", runner.calls[0], want)
			}
		})
	}
}

func TestJournalctlLogCollector_LineSplittingAndEmptyOutput(t *testing.T) {
	t.Run("splits lines and trims trailing empty newlines", func(t *testing.T) {
		runner := &stubLogCmdRunner{out: []byte("Nov 19 20:50:01 line1\r\nNov 19 20:50:02 line2\n\n")}
		collector := NewJournalctlLogCollector(runner)

		got, err := collector.Collect(context.Background(), "xray", 10)
		if err != nil {
			t.Fatalf("Collect failed: %v", err)
		}
		want := []string{"Nov 19 20:50:01 line1", "Nov 19 20:50:02 line2"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("empty output returns empty slice", func(t *testing.T) {
		runner := &stubLogCmdRunner{out: []byte("\n\n")}
		collector := NewJournalctlLogCollector(runner)

		got, err := collector.Collect(context.Background(), "xray", 10)
		if err != nil {
			t.Fatalf("Collect failed: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected 0 lines, got %v", got)
		}
	})
}

func TestJournalctlLogCollector_CommandFailureReturnsError(t *testing.T) {
	boom := errors.New("exit status 1")
	runner := &stubLogCmdRunner{
		out: []byte("Failed to get journal"),
		err: boom,
	}
	collector := NewJournalctlLogCollector(runner)

	_, err := collector.Collect(context.Background(), "xray", 10)
	if err == nil {
		t.Fatal("expected error when command fails")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected wrapped error %v, got %v", boom, err)
	}
	if !strings.Contains(err.Error(), "Failed to get journal") {
		t.Fatalf("expected command output in error, got %v", err)
	}
}
