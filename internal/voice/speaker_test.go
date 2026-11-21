package voice

import "testing"

func TestSentenceChunker(t *testing.T) {
	var c SentenceChunker
	got := c.Push("I found the bug in `ScoreboardView.swift`. ")
	if len(got) != 1 || got[0] != "I found the bug in ScoreboardView.swift." {
		t.Fatalf("unexpected chunk: %#v", got)
	}
	got2 := c.Push("Updating player one color now")
	if len(got2) != 0 {
		t.Fatalf("expected no chunk yet, got %#v", got2)
	}
	if rem := c.Flush(); rem != "Updating player one color now" {
		t.Fatalf("unexpected flush: %q", rem)
	}
}
