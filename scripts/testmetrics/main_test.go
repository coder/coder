package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParseEvents(t *testing.T) {
	t.Parallel()
	data := strings.Join([]string{
		`{"Action":"run","Package":"example.test","Test":"TestSlow"}`,
		`{"Action":"output","Package":"example.test","Test":"TestSlow","Output":"CODER_TESTMETRICS eventually_ns=1200000 test=TestSlow\n"}`,
		`{"Action":"pass","Package":"example.test","Test":"TestSlow","Elapsed":0.25}`,
	}, "\n")

	rows, err := parseEvents([]byte(data), testCase{Package: "example.test", Name: "TestSlow"}, resources{UserCPU: time.Millisecond, Available: true}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row.Test != "TestSlow" || row.Status != "pass" || row.Wall != 250*time.Millisecond {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.EventuallyWait != 1200*time.Microsecond || row.EventuallyCallCount != 1 {
		t.Fatalf("unexpected wait metrics: %+v", row)
	}
}

func TestParseEventsIncludesSubtests(t *testing.T) {
	t.Parallel()
	data := "{" +
		`"Action":"pass","Package":"example.test","Test":"TestParent/sub","Elapsed":0.1` +
		"}\n"
	rows, err := parseEvents([]byte(data), testCase{Package: "example.test", Name: "TestParent"}, resources{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Test != "TestParent/sub" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestWriteCSV(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	rows := []metrics{{
		Package:             "example.test",
		Test:                "Test,quoted",
		Status:              "pass",
		Wall:                2 * time.Second,
		UserCPU:             500 * time.Millisecond,
		SystemCPU:           100 * time.Millisecond,
		MemoryBytes:         123,
		ResourceAvailable:   true,
		EventuallyWait:      250 * time.Millisecond,
		EventuallyCallCount: 2,
	}}
	if err := writeCSV("-", &output, rows); err != nil {
		t.Fatal(err)
	}
	want := "package,test,status,wall_ms,user_cpu_ms,system_cpu_ms,wall_minus_cpu_ms,memory_peak_bytes,eventually_wait_ms,eventually_calls,resource_scope\n" +
		"example.test,\"Test,quoted\",pass,2000.000,500.000,100.000,1400.000,123,250.000,2,test-process\n"
	if output.String() != want {
		t.Fatalf("CSV mismatch:\ngot:\n%s\nwant:\n%s", output.String(), want)
	}
}

func TestParseWaitMarker(t *testing.T) {
	t.Parallel()
	waits := make(map[string]struct {
		wait  time.Duration
		calls int64
	})
	parseWaitMarker("CODER_TESTMETRICS eventually_ns=7 test=Test/sub\n", waits)
	parseWaitMarker("CODER_TESTMETRICS eventually_ns=3 test=Test/sub\n", waits)
	got := waits["Test/sub"]
	if got.wait != 10*time.Nanosecond || got.calls != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestRunCommand(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("sh", "-c", "exit 7")
	if err := runCommand(cmd, time.Second); err == nil {
		t.Fatal("runCommand returned nil for failed command")
	}
}
