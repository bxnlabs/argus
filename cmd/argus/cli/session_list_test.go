package cli

import (
	"fmt"
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	sqliteLayout := "2006-01-02 15:04:05"

	tests := []struct {
		name   string
		offset time.Duration
		layout string
		want   string
	}{
		{
			name:   "just now",
			offset: 30 * time.Second,
			layout: sqliteLayout,
			want:   "just now",
		},
		{
			name:   "singular minute",
			offset: 90 * time.Second,
			layout: sqliteLayout,
			want:   "1 min ago",
		},
		{
			name:   "plural minutes",
			offset: 30 * time.Minute,
			layout: sqliteLayout,
			want:   "30 min ago",
		},
		{
			name:   "singular hour",
			offset: 90 * time.Minute,
			layout: sqliteLayout,
			want:   "1 hour ago",
		},
		{
			name:   "plural hours",
			offset: 5 * time.Hour,
			layout: sqliteLayout,
			want:   "5 hours ago",
		},
		{
			name:   "singular day",
			offset: 36 * time.Hour,
			layout: sqliteLayout,
			want:   "1 day ago",
		},
		{
			name:   "plural days",
			offset: 3 * 24 * time.Hour,
			layout: sqliteLayout,
			want:   "3 days ago",
		},
		{
			name:   "RFC3339 layout - just now",
			offset: 30 * time.Second,
			layout: time.RFC3339,
			want:   "just now",
		},
		{
			name:   "RFC3339 layout - plural hours",
			offset: 5 * time.Hour,
			layout: time.RFC3339,
			want:   "5 hours ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := time.Now().UTC().Add(-tt.offset).Format(tt.layout)
			got := relativeTime(ts)
			if got != tt.want {
				t.Errorf("relativeTime(%q) = %q, want %q", ts, got, tt.want)
			}
		})
	}
}

func TestRelativeTime_Fallback(t *testing.T) {
	inputs := []string{
		"not-a-date",
		"2006/01/02",
		"01-02-2006 15:04:05",
		"",
	}
	for _, ts := range inputs {
		t.Run(fmt.Sprintf("input=%q", ts), func(t *testing.T) {
			got := relativeTime(ts)
			if got != ts {
				t.Errorf("relativeTime(%q) = %q, want raw input %q", ts, got, ts)
			}
		})
	}
}
