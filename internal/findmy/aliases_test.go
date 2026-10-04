package findmy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAliasesRejectsMalformedConfigWithoutChangingIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, invalid := range []string{`{"phone":"My iPhone",`, `null`, `[]`, `{"phone":42}`} {
		if err := os.MkdirAll(filepath.Dir(aliasPath()), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(aliasPath(), []byte(invalid), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadAliases(); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
		if _, err := ResolveAlias("phone"); err == nil {
			t.Fatalf("resolved malformed %q", invalid)
		}
		actual, err := os.ReadFile(aliasPath())
		if err != nil || string(actual) != invalid {
			t.Fatalf("config changed: %q err=%v", actual, err)
		}
	}
}

func TestAliasesMissingAndRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	aliases, err := LoadAliases()
	if err != nil || len(aliases) != 0 {
		t.Fatalf("aliases=%v err=%v", aliases, err)
	}
	if err := SaveAliases(map[string]string{"Phone": "My iPhone"}); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveAlias("phone")
	if err != nil || got != "My iPhone" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
