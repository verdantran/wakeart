package scene

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/verdantran/wakeart/internal/proc"
)

// A kind this build understands but this machine cannot run is not a broken
// scene. It must vanish from the deck without a warning the user cannot act
// on — and it must appear wherever it does work. This asserts both, by asking
// the builder which case it is on the machine running the test.
func TestUnavailableKindIsDroppedNotReported(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname = \"Spectrum\"\nkind = \"spectrum\"\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "spectrum"+Ext), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, buildErr := proc.Build(proc.Params{Kind: "spectrum"})
	available := buildErr == nil

	reg := Load(nil, []string{dir})
	s, _ := reg.Find("spectrum")

	if available && s == nil {
		t.Error("the scene builds on this machine but is missing from the deck")
	}
	if !available && s != nil {
		t.Error("the scene cannot run on this machine but is in the deck anyway")
	}
	for _, e := range reg.Errs {
		t.Errorf("an unavailable kind was reported as an error: %v", e)
	}
}

// A genuinely malformed scene must still be reported, or the silent-drop path
// would swallow real mistakes.
func TestMalformedSceneIsStillReported(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname = \"Broken\"\nkind = \"no-such-kind\"\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "broken"+Ext), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := Load(nil, []string{dir})
	if len(reg.Errs) == 0 {
		t.Error("an unknown kind was dropped silently")
	}
	if s, _ := reg.Find("broken"); s != nil {
		t.Error("a scene that does not parse is in the deck")
	}
}
