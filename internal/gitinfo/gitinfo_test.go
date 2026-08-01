package gitinfo

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		out    string
		branch string
		dirty  int
	}{
		{"## main...origin/main\n M a.go\n?? b.go\n", "main", 2},
		{"## main...origin/main [ahead 1]\n", "main", 0},
		{"## feature/x\nM  y.go\n", "feature/x", 0 + 1},
		{"## HEAD (no branch)\n", "HEAD", 0},
	}
	for _, c := range cases {
		got := parse(c.out)
		if !got.IsRepo || got.Branch != c.branch || got.Dirty != c.dirty {
			t.Errorf("parse(%q) = %+v, want branch=%q dirty=%d", c.out, got, c.branch, c.dirty)
		}
	}
}

func TestLoadNonRepo(t *testing.T) {
	if st := Load(t.TempDir()); st.IsRepo {
		t.Error("temp dir should not be a repo")
	}
}
