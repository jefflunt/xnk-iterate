package capture

import "testing"

func TestListTargetsReturnsAtLeastOneDisplay(t *testing.T) {
	c := NewPlatformCapturer()
	targets, err := c.ListTargets()
	if err != nil {
		t.Fatalf("ListTargets failed: %v", err)
	}
	if len(targets) == 0 {
		t.Fatalf("expected at least one display target")
	}
}
