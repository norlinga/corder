package jobs

import "testing"

func TestUpdateFinishedAndDisplayStatus(t *testing.T) {
	running := Update{Message: "Converting", Percent: 37, Status: StatusRunning}
	if running.Finished() {
		t.Fatal("running update reported finished")
	}
	if got := running.DisplayStatus(); got != "Converting 37%" {
		t.Fatalf("running display = %q, want Converting 37%%", got)
	}

	done := Update{Message: "Saved", Percent: 100, Status: StatusDone}
	if !done.Finished() {
		t.Fatal("done update did not report finished")
	}
	if got := done.DisplayStatus(); got != "Saved" {
		t.Fatalf("done display = %q, want Saved", got)
	}
}

func TestTracker(t *testing.T) {
	tracker := NewTracker()
	update := Update{ID: "recording.wav", Kind: KindConversion, Status: StatusQueued}

	tracker.Set(update)
	got, ok := tracker.Get(update.ID)
	if !ok || got != update {
		t.Fatalf("Get = %+v, %t; want %+v, true", got, ok, update)
	}
	if tracker.Len() != 1 {
		t.Fatalf("Len = %d, want 1", tracker.Len())
	}

	tracker.Delete(update.ID)
	if _, ok := tracker.Get(update.ID); ok {
		t.Fatal("deleted update still present")
	}
	if tracker.Len() != 0 {
		t.Fatalf("Len after delete = %d, want 0", tracker.Len())
	}
}

func TestTrackerDefaultsNamespacedID(t *testing.T) {
	tracker := NewTracker()
	update := Update{Kind: KindConversion, Path: "/recordings/a.wav", Status: StatusQueued}

	tracker.Set(update)

	got, ok := tracker.Get(ID(KindConversion, "/recordings/a.wav"))
	if !ok {
		t.Fatal("namespaced update not found")
	}
	if got.ID != ID(KindConversion, "/recordings/a.wav") {
		t.Fatalf("ID = %q", got.ID)
	}
}
