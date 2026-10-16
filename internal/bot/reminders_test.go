package bot

import (
	"testing"
	"time"
)

func TestNextDay(t *testing.T) {
	msk, _ := time.LoadLocation("Europe/Moscow")
	now := time.Date(2026, 10, 15, 23, 50, 0, 0, msk)
	cases := []struct {
		in, want time.Time
	}{
		// A deadline just after midnight moves a full day, not to "tomorrow" which is the same day.
		{time.Date(2026, 10, 16, 0, 15, 0, 0, msk), time.Date(2026, 10, 17, 0, 15, 0, 0, msk)},
		// A reminder that already fired comes back tomorrow at the same time of day.
		{time.Date(2026, 10, 15, 9, 30, 0, 0, msk), time.Date(2026, 10, 16, 9, 30, 0, 0, msk)},
		{time.Date(2026, 10, 10, 18, 0, 0, 0, msk), time.Date(2026, 10, 16, 18, 0, 0, 0, msk)},
	}
	for _, c := range cases {
		if got := nextDay(c.in, now, msk); !got.Equal(c.want) {
			t.Errorf("nextDay(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
