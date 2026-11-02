package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	corev2 "github.com/sensu/core/v2"
	"github.com/sensu/sensu-plugin-sdk/sensu"
)

// Config represents the check plugin config.
type Config struct {
	sensu.PluginConfig
	ChronycCmd string
	WarnOffset float64
	CritOffset float64
	WarnAge    int
	CritAge    int
	Timeout    int
}

var (
	plugin = Config{
		PluginConfig: sensu.PluginConfig{
			Name:     "check-chrony",
			Short:    "Check that the system clock is synchronised by chrony",
			Keyspace: "sensu.io/plugins/check-chrony/config",
		},
	}

	options = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      "chronyc-cmd",
			Env:       "CHECK_CHRONY_CHRONYC_CMD",
			Argument:  "chronyc-cmd",
			Shorthand: "c",
			Default:   "chronyc",
			Usage:     "Path to the chronyc binary",
			Value:     &plugin.ChronycCmd,
		},
		&sensu.PluginConfigOption[float64]{
			Path:     "warn-offset",
			Env:      "CHECK_CHRONY_WARN_OFFSET",
			Argument: "warn-offset",
			Default:  50,
			Usage:    "Warning if the system clock is off by this many milliseconds or more",
			Value:    &plugin.WarnOffset,
		},
		&sensu.PluginConfigOption[float64]{
			Path:     "crit-offset",
			Env:      "CHECK_CHRONY_CRIT_OFFSET",
			Argument: "crit-offset",
			Default:  100,
			Usage:    "Critical if the system clock is off by this many milliseconds or more",
			Value:    &plugin.CritOffset,
		},
		&sensu.PluginConfigOption[int]{
			Path:     "warn-age",
			Env:      "CHECK_CHRONY_WARN_AGE",
			Argument: "warn-age",
			Default:  3600,
			Usage:    "Warning if the last clock update is this many seconds old or more",
			Value:    &plugin.WarnAge,
		},
		&sensu.PluginConfigOption[int]{
			Path:     "crit-age",
			Env:      "CHECK_CHRONY_CRIT_AGE",
			Argument: "crit-age",
			Default:  14400,
			Usage:    "Critical if the last clock update is this many seconds old or more",
			Value:    &plugin.CritAge,
		},
		&sensu.PluginConfigOption[int]{
			Path:     "timeout",
			Env:      "CHECK_CHRONY_TIMEOUT",
			Argument: "timeout",
			Default:  10,
			Usage:    "chronyc timeout in seconds",
			Value:    &plugin.Timeout,
		},
	}
)

func main() {
	check := sensu.NewGoCheck(&plugin.PluginConfig, options, checkArgs, executeCheck, false)
	check.Execute()
}

func checkArgs(event *corev2.Event) (int, error) {
	if err := thresholds().validate(); err != nil {
		return sensu.CheckStateUnknown, err
	}
	if plugin.Timeout <= 0 {
		return sensu.CheckStateUnknown, errors.New("--timeout must be greater than 0")
	}
	return sensu.CheckStateOK, nil
}

func executeCheck(event *corev2.Event) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(plugin.Timeout)*time.Second)
	defer cancel()

	// -n: no DNS lookup, -c: CSV output with fixed fields.
	cmd := exec.CommandContext(ctx, plugin.ChronycCmd, "-n", "-c", "tracking")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return report(Result{sensu.CheckStateCritical, fmt.Sprintf("chronyd did not answer within %ds", plugin.Timeout)})
	case errors.As(err, &exitErr):
		// chronyd stopped or unreachable ("506 Cannot talk to daemon").
		msg := strings.TrimSpace(stderr.String() + " " + stdout.String())
		return report(Result{sensu.CheckStateCritical, fmt.Sprintf("chronyc tracking failed: %s", msg)})
	case err != nil:
		return sensu.CheckStateUnknown, fmt.Errorf("running %s: %w", plugin.ChronycCmd, err)
	}

	tracking, err := parseTracking(stdout.String())
	if err != nil {
		return sensu.CheckStateUnknown, err
	}
	return report(evaluate(tracking, thresholds(), time.Now()))
}

func report(result Result) (int, error) {
	fmt.Printf("%s %s: %s\n", plugin.Name, stateLabel(result.State), result.Message)
	return result.State, nil
}

func thresholds() Thresholds {
	return Thresholds{
		WarnOffset: plugin.WarnOffset,
		CritOffset: plugin.CritOffset,
		WarnAge:    time.Duration(plugin.WarnAge) * time.Second,
		CritAge:    time.Duration(plugin.CritAge) * time.Second,
	}
}

// Tracking holds the fields of `chronyc -c tracking` used by the check.
type Tracking struct {
	Source  string
	Stratum int
	// RefTime is the time of the last clock update, zero if the clock was never synchronised.
	RefTime time.Time
	// Offset is the current offset of the system clock in seconds, positive when it is slow.
	Offset float64
	Leap   string
}

const notSynchronised = "Not synchronised"

// parseTracking parses the CSV output of `chronyc -c tracking`: reference ID, reference name,
// stratum, ref time, system time, last offset, RMS offset, frequency, residual frequency,
// skew, root delay, root dispersion, update interval, leap status.
func parseTracking(out string) (Tracking, error) {
	line := strings.TrimSpace(out)
	fields := strings.Split(line, ",")
	if len(fields) != 14 {
		return Tracking{}, fmt.Errorf("unexpected chronyc output: %q", line)
	}

	stratum, err := strconv.Atoi(fields[2])
	if err != nil {
		return Tracking{}, fmt.Errorf("invalid stratum %q", fields[2])
	}
	refTime, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return Tracking{}, fmt.Errorf("invalid ref time %q", fields[3])
	}
	offset, err := strconv.ParseFloat(fields[4], 64)
	if err != nil {
		return Tracking{}, fmt.Errorf("invalid system time offset %q", fields[4])
	}

	t := Tracking{Source: fields[1], Stratum: stratum, Offset: offset, Leap: fields[13]}
	if refTime > 0 {
		sec, frac := math.Modf(refTime)
		t.RefTime = time.Unix(int64(sec), int64(frac*1e9))
	}
	return t, nil
}

// Thresholds of the check: offsets in milliseconds, ages of the last clock update.
type Thresholds struct {
	WarnOffset, CritOffset float64
	WarnAge, CritAge       time.Duration
}

func (th Thresholds) validate() error {
	switch {
	case th.WarnOffset <= 0 || th.CritOffset <= 0:
		return errors.New("--warn-offset and --crit-offset must be greater than 0")
	case th.WarnOffset > th.CritOffset:
		return errors.New("--warn-offset must not be greater than --crit-offset")
	case th.WarnAge <= 0 || th.CritAge <= 0:
		return errors.New("--warn-age and --crit-age must be greater than 0")
	case th.WarnAge > th.CritAge:
		return errors.New("--warn-age must not be greater than --crit-age")
	}
	return nil
}

// Result is the outcome of the evaluation of the tracking data.
type Result struct {
	State   int
	Message string
}

// evaluate turns the chrony tracking data into a check result.
func evaluate(t Tracking, th Thresholds, now time.Time) Result {
	if t.Leap == notSynchronised || t.RefTime.IsZero() {
		return Result{sensu.CheckStateCritical, "clock not synchronised (no usable NTP source)"}
	}

	state := sensu.CheckStateOK

	offsetMs := math.Abs(t.Offset) * 1000
	direction := "slow"
	if t.Offset < 0 {
		direction = "fast"
	}
	offsetMsg := fmt.Sprintf("offset %.3fms %s", offsetMs, direction)
	switch {
	case offsetMs >= th.CritOffset:
		state = sensu.CheckStateCritical
		offsetMsg += fmt.Sprintf(" (critical >= %gms)", th.CritOffset)
	case offsetMs >= th.WarnOffset:
		state = sensu.CheckStateWarning
		offsetMsg += fmt.Sprintf(" (warning >= %gms)", th.WarnOffset)
	}

	age := max(now.Sub(t.RefTime), 0).Round(time.Second)
	ageMsg := fmt.Sprintf("last sync %s ago", age)
	switch {
	case age >= th.CritAge:
		state = sensu.CheckStateCritical
		ageMsg += fmt.Sprintf(" (critical >= %s)", th.CritAge)
	case age >= th.WarnAge:
		state = max(state, sensu.CheckStateWarning)
		ageMsg += fmt.Sprintf(" (warning >= %s)", th.WarnAge)
	}

	return Result{state, fmt.Sprintf("synchronised to %s (stratum %d), %s, %s", t.Source, t.Stratum, offsetMsg, ageMsg)}
}

func stateLabel(state int) string {
	switch state {
	case sensu.CheckStateOK:
		return "OK"
	case sensu.CheckStateWarning:
		return "WARNING"
	case sensu.CheckStateCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}
