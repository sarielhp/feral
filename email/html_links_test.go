package email

import (
	"strings"
	"testing"
)

func TestMarkAnchorEndsPicksAnUnusedMarker(t *testing.T) {
	marked, marker := markAnchorEnds(`<p>mlnkend <a href="https://a.example/">A</a><a href="https://b.example/">B</A ></p>`)
	if marker != "mlnkendz" {
		t.Errorf("marker = %q, want one the source does not contain", marker)
	}
	want := `<p>mlnkend <a href="https://a.example/">A` + marker + `</a><a href="https://b.example/">B` + marker + `</A ></p>`
	if marked != want {
		t.Errorf("marked = %q\nwant     %q", marked, want)
	}
}

// labelLinks works on w3m's dump, which these inputs imitate, so the shape of
// the result does not depend on w3m being installed.
func TestLabelLinks(t *testing.T) {
	const mark = "mlnkend"
	refs := "\n\nReferences:\n\n[1] https://login.example/magic#tok\n[2] https://t.example/c?u=1\n[3] https://urldefense.com/v3/__https://social.example/x__;!!a!b$\n[4] https://t.example/c?u=4\n[5] mailto:help@example.com"
	tests := []struct {
		name, body, want string
	}{
		{"button", "Click below.\n   [1]Sign in" + mark + "\n", "Click below.\n   [Sign in][1]\n"},
		{"inline text keeps its punctuation", "Contact [2]Example Support" + mark + ".", "Contact [Example Support][2]."},
		{"label wrapped by w3m is one line", "[1]Sign in to\n   your account" + mark, "[Sign in to your account][1]"},
		{"image placeholder is named by its host, unwrapped", "[3]\u25cf" + mark, "[social.example][3]"},
		{"brackets and quotes cannot end the link early", `[4]see ["this"]` + mark, "[see ('this')][4]"},
		{"image placeholder named by w3m keeps the name", "[4][x]" + mark, "[x][4]"},
		{"mailto", "[5]Help" + mark, "[Help][5]"},
		{"anchor with no end keeps w3m's form", "[1]Sign in\nmore text", "[1]Sign in\nmore text"},
		{"number with no target is left alone", "[9]Nine" + mark, "[9]Nine"},
		{"stray marker is dropped", "plain" + mark + " text", "plain text"},
		{"a citation before an anchor is not the anchor", "see [2] and [1]Go" + mark, "see [2] and [Go][1]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := labelLinks(tc.body+refs, mark)
			body, _, ok := strings.Cut(got, referencesHeading)
			if !ok {
				t.Fatalf("References list lost:\n%s", got)
			}
			if body != tc.want {
				t.Errorf("body = %q\nwant   %q", body, tc.want)
			}
		})
	}
}

func TestLabelLinksTitlesTheReferences(t *testing.T) {
	const mark = "mlnkend"
	in := "[1]Sign in" + mark + " and [3]\u25cf" + mark + "\n\nReferences:\n\n[1] https://login.example/magic#tok\n[2] https://t.example/c?u=2\n[3] https://social.example/x"
	got := labelLinks(in, mark)
	want := "[Sign in][1] and [social.example][3]\n\nReferences:\n\n[1]: https://login.example/magic#tok \"Sign in\"\n[2]: https://t.example/c?u=2\n[3]: https://social.example/x \"social.example\""
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLabelLinksWithoutReferences(t *testing.T) {
	if got := labelLinks("no links here mlnkend", "mlnkend"); got != "no links here " {
		t.Errorf("got %q", got)
	}
}
