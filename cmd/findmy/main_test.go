package main

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/oshahine/findmy-cli/internal/findmy"
)

func TestParseWatchOptsDefaultsDiffForJSONOnly(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantJSON bool
		wantDiff bool
	}{
		{
			name:     "human output defaults diff off",
			args:     []string{"people"},
			wantJSON: false,
			wantDiff: false,
		},
		{
			name:     "json output defaults diff on",
			args:     []string{"people", "--json"},
			wantJSON: true,
			wantDiff: true,
		},
		{
			name:     "json output can request unchanged heartbeats",
			args:     []string{"people", "--json", "--no-diff"},
			wantJSON: true,
			wantDiff: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseWatchOpts(tt.args)
			if err != nil {
				t.Fatalf("parseWatchOpts returned error: %v", err)
			}
			if opts.JSON != tt.wantJSON {
				t.Fatalf("JSON = %v, want %v", opts.JSON, tt.wantJSON)
			}
			if opts.Diff != tt.wantDiff {
				t.Fatalf("Diff = %v, want %v", opts.Diff, tt.wantDiff)
			}
		})
	}
}

func TestExecutePlaySoundDryRunNeverCallsAction(t *testing.T) {
	called := false
	result, err := executePlaySound(
		playSoundOpts{device: "Phone"},
		func() ([]findmy.Device, error) { return []findmy.Device{{Name: "Phone"}}, nil },
		func(findmy.Device) error { called = true; return nil },
	)
	if err != nil {
		t.Fatalf("executePlaySound returned error: %v", err)
	}
	if called {
		t.Fatal("action was called without confirmation")
	}
	if !result.DryRun || result.Confirmed || !result.OK {
		t.Fatalf("result = %#v", result)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"ok":true,"action":"play-sound","device":"Phone","confirmed":false,"dry_run":true}` {
		t.Fatalf("JSON = %s", raw)
	}
}

func TestExecutePlaySoundConfirmedCallsAction(t *testing.T) {
	called := false
	result, err := executePlaySound(
		playSoundOpts{device: "Phone", confirm: true},
		func() ([]findmy.Device, error) { return []findmy.Device{{Name: "Phone"}}, nil },
		func(device findmy.Device) error { called = device.Name == "Phone"; return nil },
	)
	if err != nil || !called || !result.OK || result.DryRun || !result.Confirmed {
		t.Fatalf("result = %#v, called = %v, err = %v", result, called, err)
	}
}

func TestExecutePlaySoundPassesCanonicalExactNameToAction(t *testing.T) {
	var got string
	_, err := executePlaySound(
		playSoundOpts{device: "ipad", confirm: true},
		func() ([]findmy.Device, error) { return []findmy.Device{{Name: "Omar's iPad"}}, nil },
		func(device findmy.Device) error { got = device.Name; return nil },
	)
	if err != nil {
		t.Fatalf("executePlaySound returned error: %v", err)
	}
	if got != "Omar's iPad" {
		t.Fatalf("action target = %q, want canonical exact name", got)
	}
}

func TestExecutePlaySoundReturnsActionError(t *testing.T) {
	want := errors.New("AX action failed")
	_, err := executePlaySound(
		playSoundOpts{device: "Phone", confirm: true},
		func() ([]findmy.Device, error) { return []findmy.Device{{Name: "Phone"}}, nil },
		func(findmy.Device) error { return want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestExecutePlaySoundAmbiguousNeverCallsAction(t *testing.T) {
	called := false
	_, err := executePlaySound(
		playSoundOpts{device: "Phone", confirm: true},
		func() ([]findmy.Device, error) {
			return []findmy.Device{{Name: "Work Phone"}, {Name: "Home Phone"}}, nil
		},
		func(findmy.Device) error { called = true; return nil },
	)
	if err == nil {
		t.Fatal("ambiguous target returned nil error")
	}
	if called {
		t.Fatal("action was called for an ambiguous target")
	}
}

func TestParsePlaySoundOpts(t *testing.T) {
	opts, err := parsePlaySoundOpts([]string{"My", "Phone", "--json", "--confirm"})
	if err != nil || opts.device != "My Phone" || !opts.json || !opts.confirm {
		t.Fatalf("opts = %#v, err = %v", opts, err)
	}
	if _, err := parsePlaySoundOpts([]string{"Phone", "--force"}); err == nil {
		t.Fatal("unknown flag was accepted")
	}
}

func TestParseWatchOptsIntervalKindAndOnce(t *testing.T) {
	opts, err := parseWatchOpts([]string{"--interval=10s", "items", "--once", "--diff"})
	if err != nil {
		t.Fatalf("parseWatchOpts returned error: %v", err)
	}
	if opts.Kind != findmy.WatchItems {
		t.Fatalf("Kind = %q, want %q", opts.Kind, findmy.WatchItems)
	}
	if opts.Interval != 10*time.Second {
		t.Fatalf("Interval = %s, want 10s", opts.Interval)
	}
	if !opts.Once {
		t.Fatal("Once = false, want true")
	}
	if !opts.Diff {
		t.Fatal("Diff = false, want true")
	}
}

func TestParseWatchOptsRejectsNonPositiveInterval(t *testing.T) {
	for _, interval := range []string{"0s", "-1s"} {
		t.Run(interval, func(t *testing.T) {
			if _, err := parseWatchOpts([]string{"people", "--interval=" + interval}); err == nil {
				t.Fatal("parseWatchOpts returned nil error")
			}
		})
	}
}
