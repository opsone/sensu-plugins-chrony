package main

import (
	"testing"
	"time"

	"github.com/sensu/sensu-plugin-sdk/sensu"
)

const (
	syncedOutput   = "AC120002,172.18.0.2,4,1793637302.540862724,0.150027901,0.000013283,0.000015662,-3.136,0.785,12.448,0.299808651,0.000037704,1.0,Normal\n"
	unsyncedOutput = "00000000,,0,0.000000000,0.000000000,0.000000000,0.000000000,0.000,0.000,0.000,1.000000000,1.000000000,0.0,Not synchronised\n"
)

func TestParseTracking(t *testing.T) {
	got, err := parseTracking(syncedOutput)
	if err != nil {
		t.Fatal(err)
	}
	want := Tracking{
		Source:  "172.18.0.2",
		Stratum: 4,
		RefTime: time.Unix(1793637302, 540862724),
		Offset:  0.150027901,
		Leap:    "Normal",
	}
	if got.Source != want.Source || got.Stratum != want.Stratum || got.Offset != want.Offset || got.Leap != want.Leap ||
		got.RefTime.Sub(want.RefTime).Abs() > time.Microsecond {
		t.Errorf("parseTracking() = %+v, want %+v", got, want)
	}

	got, err = parseTracking(unsyncedOutput)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RefTime.IsZero() || got.Leap != notSynchronised {
		t.Errorf("parseTracking(unsynced) = %+v", got)
	}

	for _, bad := range []string{
		"",
		"506 Cannot talk to daemon",
		"AC120002,x,four,1.0,0.1,0,0,0,0,0,0,0,1.0,Normal",
		"AC120002,x,4,now,0.1,0,0,0,0,0,0,0,1.0,Normal",
		"AC120002,x,4,1.0,slow,0,0,0,0,0,0,0,1.0,Normal",
	} {
		if _, err := parseTracking(bad); err == nil {
			t.Errorf("parseTracking(%q): expected an error", bad)
		}
	}
}

func TestEvaluate(t *testing.T) {
	now := time.Unix(1793640000, 0)
	th := Thresholds{WarnOffset: 50, CritOffset: 100, WarnAge: time.Hour, CritAge: 4 * time.Hour}
	synced := func(offset float64, age time.Duration) Tracking {
		return Tracking{Source: "10.0.0.1", Stratum: 3, RefTime: now.Add(-age), Offset: offset, Leap: "Normal"}
	}

	tests := []struct {
		name string
		t    Tracking
		want Result
	}{
		{
			name: "synchronised",
			t:    synced(0.000012, 64*time.Second),
			want: Result{sensu.CheckStateOK, "synchronised to 10.0.0.1 (stratum 3), offset 0.012ms slow, last sync 1m4s ago"},
		},
		{
			name: "fast clock below warning",
			t:    synced(-0.0499, time.Second),
			want: Result{sensu.CheckStateOK, "synchronised to 10.0.0.1 (stratum 3), offset 49.900ms fast, last sync 1s ago"},
		},
		{
			name: "offset warning",
			t:    synced(-0.05, time.Second),
			want: Result{sensu.CheckStateWarning, "synchronised to 10.0.0.1 (stratum 3), offset 50.000ms fast (warning >= 50ms), last sync 1s ago"},
		},
		{
			name: "offset critical",
			t:    synced(0.150028, time.Second),
			want: Result{sensu.CheckStateCritical, "synchronised to 10.0.0.1 (stratum 3), offset 150.028ms slow (critical >= 100ms), last sync 1s ago"},
		},
		{
			name: "stale warning",
			t:    synced(0.000001, 2*time.Hour),
			want: Result{sensu.CheckStateWarning, "synchronised to 10.0.0.1 (stratum 3), offset 0.001ms slow, last sync 2h0m0s ago (warning >= 1h0m0s)"},
		},
		{
			name: "stale critical",
			t:    synced(0.000001, 5*time.Hour),
			want: Result{sensu.CheckStateCritical, "synchronised to 10.0.0.1 (stratum 3), offset 0.001ms slow, last sync 5h0m0s ago (critical >= 4h0m0s)"},
		},
		{
			name: "offset critical wins over stale warning",
			t:    synced(0.2, 2*time.Hour),
			want: Result{sensu.CheckStateCritical, "synchronised to 10.0.0.1 (stratum 3), offset 200.000ms slow (critical >= 100ms), last sync 2h0m0s ago (warning >= 1h0m0s)"},
		},
		{
			name: "ref time in the future",
			t:    synced(0, -time.Minute),
			want: Result{sensu.CheckStateOK, "synchronised to 10.0.0.1 (stratum 3), offset 0.000ms slow, last sync 0s ago"},
		},
		{
			name: "not synchronised",
			t:    Tracking{Leap: notSynchronised},
			want: Result{sensu.CheckStateCritical, "clock not synchronised (no usable NTP source)"},
		},
		{
			name: "never updated",
			t:    Tracking{Source: "10.0.0.1", Stratum: 3, Leap: "Normal"},
			want: Result{sensu.CheckStateCritical, "clock not synchronised (no usable NTP source)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evaluate(tt.t, th, now); got != tt.want {
				t.Errorf("evaluate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestThresholdsValidate(t *testing.T) {
	valid := Thresholds{WarnOffset: 50, CritOffset: 100, WarnAge: time.Hour, CritAge: 4 * time.Hour}
	if err := valid.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	equal := Thresholds{WarnOffset: 100, CritOffset: 100, WarnAge: time.Hour, CritAge: time.Hour}
	if err := equal.validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	for name, th := range map[string]Thresholds{
		"zero offset":     {WarnOffset: 0, CritOffset: 100, WarnAge: time.Hour, CritAge: 4 * time.Hour},
		"warn > crit":     {WarnOffset: 200, CritOffset: 100, WarnAge: time.Hour, CritAge: 4 * time.Hour},
		"negative age":    {WarnOffset: 50, CritOffset: 100, WarnAge: -time.Hour, CritAge: 4 * time.Hour},
		"age warn > crit": {WarnOffset: 50, CritOffset: 100, WarnAge: 5 * time.Hour, CritAge: 4 * time.Hour},
	} {
		if err := th.validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
