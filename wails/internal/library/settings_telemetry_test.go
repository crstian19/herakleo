package library

import "testing"

func TestTelemetrySettings(t *testing.T) {
	l := openTemp(t)
	n := 0
	newID := func() string { n++; return "id-" + string(rune('0'+n)) }

	if id, err := l.TelemetryInstall(newID); err != nil || id != "" {
		t.Fatalf("not asked yet: %q, %v", id, err)
	}
	on, off := TelemetryOn, TelemetryOff
	if _, err := l.UpdateSettings(SettingsPatch{Telemetry: &on}); err != nil {
		t.Fatal(err)
	}
	id, _ := l.TelemetryInstall(newID)
	if again, _ := l.TelemetryInstall(newID); id != "id-1" || again != id {
		t.Errorf("the ID is created once: %q then %q", id, again)
	}
	if err := l.SetLastPing(1234); err != nil {
		t.Fatal(err)
	}

	// Reopening (or the other app) keeps the answer, the ID and the last ping.
	l2, err := Open(l.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if s := l2.Settings(); s.Telemetry != on || s.InstallID != "id-1" || s.LastPing != 1234 {
		t.Errorf("persisted %+v", s)
	}

	// Saying no forgets the ID; saying yes again starts a new one.
	if _, err := l.UpdateSettings(SettingsPatch{Telemetry: &off}); err != nil {
		t.Fatal(err)
	}
	if s := l.Settings(); s.InstallID != "" || s.LastPing != 0 {
		t.Errorf("after opting out %+v", s)
	}
	if id, _ := l.TelemetryInstall(newID); id != "" {
		t.Errorf("off sends nothing: %q", id)
	}
	_, _ = l.UpdateSettings(SettingsPatch{Telemetry: &on})
	if id, _ := l.TelemetryInstall(newID); id != "id-2" {
		t.Errorf("new ID after opting in again: %q", id)
	}
}
