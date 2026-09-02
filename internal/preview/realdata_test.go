package preview

import (
	"os"
	"testing"

	"github.com/baoyudu/perch/internal/config"
	"github.com/baoyudu/perch/internal/index"
)

// TestRealData exercises Load against the local machine's actual history.
// Skipped unless PSW_REAL_DATA=1 (developer smoke test, not CI).
func TestRealData(t *testing.T) {
	if os.Getenv("PSW_REAL_DATA") != "1" {
		t.Skip("set PSW_REAL_DATA=1 to run")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	projects, err := index.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	shown := 0
	for _, p := range projects {
		if shown >= 5 {
			break
		}
		snip := Load(p)
		if snip.Text == "" {
			continue
		}
		shown++
		text := snip.Text
		if len(text) > 120 {
			text = text[:120]
		}
		t.Logf("%s [%s/%s]: %s", p.Name, snip.Agent, snip.Role, text)
	}
	if shown == 0 {
		t.Error("no previews extracted from any real project")
	}
}
