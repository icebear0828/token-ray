package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"token-ray/internal/testutil"
)

func TestMainDispatchesAntigravityStatuslineCommand(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestAntigravityStatuslineHelperProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_ANTIGRAVITY_STATUSLINE_HELPER=1",
		"TOKEN_RAY_DATA_DIR="+t.TempDir(),
	)
	cmd.Stdin = strings.NewReader(`{
		"conversation_id": "main-dispatch",
		"model": {"id": "Gemini 3.5 Flash (High)"},
		"workspace": {"project_dir": "/Users/c/token"},
		"context_window": {"current_usage": {"input_tokens": 1000, "output_tokens": 50}}
	}`)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected helper process to succeed: %v\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "Token Ray: Antigravity 1.1K tok") {
		t.Fatalf("expected Antigravity statusline output, got %q", string(output))
	}
}

func TestAntigravityStatuslineHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ANTIGRAVITY_STATUSLINE_HELPER") != "1" {
		return
	}
	os.Args = []string{"dashboard", "antigravity-statusline"}
	main()
	t.Fatal("main returned")
}

func TestRunAntigravityStatuslineInsertsIdempotentUsage(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	raw := `{
		"cwd": "/Users/c/token",
		"activeModel": "gemini-2.5-pro",
		"conversationId": "conv-1",
		"tokenUsage": {
			"promptTokenCount": 1000,
			"cachedContentTokenCount": 200,
			"candidatesTokenCount": 150,
			"thoughtsTokenCount": 50,
			"totalTokenCount": 1200
		}
	}`

	var firstOut bytes.Buffer
	code := runAntigravityStatusline(database, calc, "local", strings.NewReader(raw), &firstOut, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected first run exit 0, got %d", code)
	}
	if !strings.Contains(firstOut.String(), "Antigravity") || !strings.Contains(firstOut.String(), "1.2K tok") {
		t.Fatalf("unexpected statusline output %q", firstOut.String())
	}

	var secondOut bytes.Buffer
	code = runAntigravityStatusline(database, calc, "local", strings.NewReader(raw), &secondOut, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected second run exit 0, got %d", code)
	}

	cost, input, cached, cacheCreation, output, err := database.QueryPeriodStatsAll("local", "Antigravity")
	if err != nil {
		t.Fatal(err)
	}
	if input != 1000 || cached != 200 || cacheCreation != 0 || output != 200 {
		t.Fatalf("unexpected idempotent Antigravity stats: cost=%f input=%d cached=%d cacheCreation=%d output=%d", cost, input, cached, cacheCreation, output)
	}
	if cost <= 0 {
		t.Fatalf("expected cost from pricing table, got %f", cost)
	}
}

func TestRunAntigravityStatuslineNoUsageIsSuccessfulNoop(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)

	var out bytes.Buffer
	code := runAntigravityStatusline(database, calc, "local", strings.NewReader(`{"cwd":"/Users/c/token"}`), &out, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected no-op exit 0, got %d", code)
	}
	if got := out.String(); !strings.Contains(got, "no Antigravity usage") {
		t.Fatalf("unexpected no-op output %q", got)
	}
}

func TestRunAntigravityStatuslinePricesCurrentCLIModel(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	raw := `{
		"conversation_id": "conv-current",
		"model": {
			"id": "Gemini 3.5 Flash (High)",
			"display_name": "Gemini 3.5 Flash (High)"
		},
		"workspace": {
			"project_dir": "/Users/c/token"
		},
		"context_window": {
			"current_usage": {
				"input_tokens": 1000000,
				"cache_read_input_tokens": 100000,
				"cache_creation_input_tokens": 50000,
				"output_tokens": 100000
			}
		}
	}`

	var out bytes.Buffer
	code := runAntigravityStatusline(database, calc, "local", strings.NewReader(raw), &out, &bytes.Buffer{})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Antigravity") || !strings.Contains(out.String(), "$2.4900") {
		t.Fatalf("unexpected statusline output %q", out.String())
	}

	cost, input, cached, cacheCreation, output, err := database.QueryPeriodStatsAll("local", "Antigravity")
	if err != nil {
		t.Fatal(err)
	}
	if input != 1150000 || cached != 100000 || cacheCreation != 50000 || output != 100000 {
		t.Fatalf("unexpected current CLI stats: cost=%f input=%d cached=%d cacheCreation=%d output=%d", cost, input, cached, cacheCreation, output)
	}
	if cost < 2.4899 || cost > 2.4901 {
		t.Fatalf("expected Gemini 3.5 Flash standard cost 2.4900, got %f", cost)
	}
}

func TestRunAntigravityStatuslineReturnsAfterOneJSONValueWithoutEOF(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = writer.Close()
		_ = reader.Close()
	})

	var out bytes.Buffer
	done := make(chan int, 1)
	var once sync.Once
	go func() {
		code := runAntigravityStatusline(database, calc, "local", reader, &out, &bytes.Buffer{})
		done <- code
	}()

	raw := `{
		"cwd": "/Users/c/token",
		"model": {"id": "Gemini 3.5 Flash (High)"},
		"conversation_id": "streaming-statusline",
		"context_window": {
			"current_usage": {
				"input_tokens": 2000,
				"output_tokens": 50
			}
		}
	}`
	if _, err := writer.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}

	select {
	case code := <-done:
		once.Do(func() { _ = writer.Close() })
		if code != 0 {
			t.Fatalf("expected exit 0, got %d", code)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("statusline reader blocked waiting for EOF")
	}

	if !strings.Contains(out.String(), "Antigravity 2.0K tok") {
		t.Fatalf("unexpected statusline output %q", out.String())
	}
}
