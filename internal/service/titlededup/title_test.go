package titlededup

import (
	"testing"

	"scriberr/internal/titleutil"
)

func TestCleanTitle(t *testing.T) {
	if titlededupClean := CleanTitle("  meeting.m4a  "); titlededupClean != "meeting" {
		t.Fatalf("CleanTitle = %q", titlededupClean)
	}
	if titleutil.DedupKey("Meeting.M4A") != DedupKey("meeting") {
		t.Fatal("expected case-insensitive dedup keys to match")
	}
}
