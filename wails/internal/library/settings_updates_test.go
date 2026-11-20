package library

import "testing"

func TestUpdateSettings(t *testing.T) {
	l := openTemp(t)
	if !l.Settings().UpdatesEnabled() {
		t.Fatal("the update check is on by default")
	}
	off := UpdatesOff
	if _, err := l.UpdateSettings(SettingsPatch{Updates: &off}); err != nil {
		t.Fatal(err)
	}
	l2, err := Open(l.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if l2.Settings().UpdatesEnabled() {
		t.Fatal("turning the check off did not persist")
	}
	// A patch without the field keeps it.
	yes := true
	if s, _ := l2.UpdateSettings(SettingsPatch{RefreshOnStart: &yes}); s.UpdatesEnabled() {
		t.Fatal("an unrelated patch turned the check back on")
	}
}
