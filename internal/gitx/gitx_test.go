package gitx

import "testing"

func TestNameFromURL(t *testing.T) {
	cases := map[string]string{
		"git@github.com:klokie/repoman.git":       "repoman",
		"https://github.com/klokie/repoman.git":   "repoman",
		"https://github.com/klokie/repoman":       "repoman",
		"https://github.com/klokie/repoman/":      "repoman",
		"ssh://git@bitbucket.org/werlabs/js.git":  "js",
		"/Volumes/Emperor/git-mirrors/hermes.git": "hermes",
	}
	for url, want := range cases {
		if got := NameFromURL(url); got != want {
			t.Errorf("%s: got %q, want %q", url, got, want)
		}
	}
}
