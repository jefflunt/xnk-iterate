package model

import "testing"

func TestNormalizeHotkeyBinding(t *testing.T) {
	b, err := NormalizeHotkeyBinding(HotkeyBinding{
		KeyCode:   "Enter",
		Modifiers: HotkeyModOption,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.Display != "⌥↩" {
		t.Fatalf("expected ⌥↩, got %q", b.Display)
	}

	b2, err := NormalizeHotkeyBinding(HotkeyBinding{
		KeyCode:   "KeyS",
		Modifiers: HotkeyModControl | HotkeyModOption | HotkeyModShift | HotkeyModCmd,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b2.Display != "⌃⌥⇧⌘S" {
		t.Fatalf("expected ⌃⌥⇧⌘S, got %q", b2.Display)
	}

	// Function keys are allowed without modifiers
	b3, err := NormalizeHotkeyBinding(HotkeyBinding{KeyCode: "F9", Modifiers: 0})
	if err != nil {
		t.Fatalf("expected F9 without modifier to succeed, got %v", err)
	}
	if b3.Display != "F9" {
		t.Fatalf("expected F9, got %q", b3.Display)
	}

	// Plain letter without modifier should be rejected so global typing isn't hijacked
	if _, err := NormalizeHotkeyBinding(HotkeyBinding{KeyCode: "KeyA", Modifiers: 0}); err == nil {
		t.Fatalf("expected plain KeyA without modifier to fail validation")
	}
}
