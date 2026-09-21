package titleutil

import "testing"

func TestCleanTitleStripsExtension(t *testing.T) {
	if got := CleanTitle("meeting.m4a"); got != "meeting" {
		t.Fatalf("got %q", got)
	}
}
