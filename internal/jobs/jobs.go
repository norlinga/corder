package jobs

import "fmt"

type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

const KindConversion = "conversion"

type Update struct {
	ID          string
	Kind        string
	Path        string
	Destination string
	Percent     float64
	Message     string
	Status      Status
	Err         error
}

func (u Update) Finished() bool {
	return u.Status == StatusDone || u.Status == StatusFailed
}

func (u Update) DisplayStatus() string {
	if u.Message == "" {
		return string(u.Status)
	}
	if u.Percent > 0 && !u.Finished() {
		return fmt.Sprintf("%s %.0f%%", u.Message, u.Percent)
	}
	return u.Message
}

type Tracker struct {
	items map[string]Update
}

func NewTracker() Tracker {
	return Tracker{items: map[string]Update{}}
}

func (t *Tracker) Set(update Update) {
	t.ensure()
	t.items[update.ID] = update
}

func (t *Tracker) Delete(id string) {
	if t.items == nil {
		return
	}
	delete(t.items, id)
}

func (t *Tracker) Get(id string) (Update, bool) {
	if t.items == nil {
		return Update{}, false
	}
	update, ok := t.items[id]
	return update, ok
}

func (t *Tracker) Len() int {
	return len(t.items)
}

func (t *Tracker) ensure() {
	if t.items == nil {
		t.items = map[string]Update{}
	}
}
