package cfhome

import (
	"testing"
)

func TestSummarizeReadsTarget(t *testing.T) {
	home := t.TempDir()
	writeConfig(t, home, []byte(`{"Target":"https://api.example.com","OrganizationFields":{"Name":"org1"},"SpaceFields":{"Name":"space1"}}`), 0o600)

	summary, present, err := Summarize(home)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if !present {
		t.Fatalf("Summarize() present = false, want true")
	}
	if summary.API != "https://api.example.com" || summary.Org != "org1" || summary.Space != "space1" {
		t.Fatalf("Summarize() = %+v", summary)
	}
}

func TestSummarizeNoConfig(t *testing.T) {
	_, present, err := Summarize(t.TempDir())
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if present {
		t.Fatalf("Summarize() present = true, want false for a home with no config")
	}
}

func TestSummarizeNoTarget(t *testing.T) {
	home := t.TempDir()
	writeConfig(t, home, []byte(`{"OrganizationFields":{"Name":"org1"}}`), 0o600)

	_, present, err := Summarize(home)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if present {
		t.Fatalf("Summarize() present = true, want false when Target is empty")
	}
}
