package speech

import (
	"encoding/json"
	"testing"
)

func TestSpeechEventJSONRoundTrip(t *testing.T) {
	ev := Event{
		Type:       EventFinal,
		Text:       "The scoreboard colors are inverted.",
		Confidence: 0.96,
		Timestamp:  33.2,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var got Event
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Type != EventFinal || got.Text != ev.Text {
		t.Fatalf("unexpected round-trip: %+v", got)
	}
}
