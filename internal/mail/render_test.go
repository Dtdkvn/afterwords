package mail

import (
	"strings"
	"testing"
)

func TestRenderAllTemplateContracts(t *testing.T) {
	t.Parallel()
	data := TemplateData{
		OwnerEmail: "owner@example.com", RecipientName: "Morgan <Recipient>", TrusteeName: "Taylor & Jordan",
		SwitchName: "Letters & memories", Deadline: "August 12, 2026 at 09:00 UTC",
		CheckinURL: "https://example.test/c/token?a=1&b=2", ReleaseURL: "https://example.test/r/token?a=1&b=2",
		KitURL: "https://example.test/kit", DaysOverdue: 2, Test: true,
	}
	for _, name := range []string{"checkin_reminder", "fire_drill_owner", "overdue_notice", "recovery_kit_trustee", "release_recipient", "trustee_release"} {
		t.Run(name, func(t *testing.T) {
			htmlBody, textBody, err := Render(name, data)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(htmlBody, "<!doctype html>") || !strings.Contains(htmlBody, "Letters &amp; memories") {
				t.Fatalf("HTML template did not render and escape shared data: %s", htmlBody)
			}
			if !strings.Contains(textBody, "Letters & memories") || strings.Contains(textBody, "{{") || strings.Contains(htmlBody, "{{") {
				t.Fatalf("template left placeholders or lost plain text data: %q", textBody)
			}
		})
	}
}

func TestRenderRejectsUnknownAndUnsafeNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", ".", "../release_recipient", "missing"} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Render(name, TemplateData{}); err == nil {
				t.Fatalf("Render(%q) succeeded", name)
			}
		})
	}
}
