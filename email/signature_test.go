package email

import "testing"

func TestNormalizeSignature(t *testing.T) {
	// Keyed by subtest name: in, then the normalized signature.
	tests := map[string]struct{ in, want string }{
		"empty":                      {"", ""},
		"blank lines":                {"\n \n\t\n", ""},
		"plain":                      {"Bob\n", "Bob"},
		"crlf":                       {"Bob\r\nExample Inc.\r\n\r\n", "Bob\nExample Inc."},
		"typed delimiter dropped":    {"-- \nBob\n", "Bob"},
		"stripped delimiter dropped": {"--\nBob", "Bob"},
		"only a delimiter":           {"-- \n", ""},
		"trailing spaces":            {"Bob  \n\n", "Bob"},
		"leading blank lines":        {"\n\nBob", "Bob"},
		"inner blank line kept":      {"Bob\n\nhttps://example.org", "Bob\n\nhttps://example.org"},
		"non-ascii":                  {"רועי\nשלום\n", "רועי\nשלום"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := NormalizeSignature(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeSignature(%q) = %q, want %q", tt.in, got, tt.want)
			}
			// Normalizing is idempotent, and only a blank signature has no block.
			if again := NormalizeSignature(got); again != got {
				t.Errorf("normalizing %q again gave %q", got, again)
			}
			if blank := SignatureBlock(tt.in) == ""; blank != (tt.want == "") {
				t.Errorf("SignatureBlock(%q) blank = %v, want %v", tt.in, blank, tt.want == "")
			}
		})
	}
}

func TestSignatureBlock(t *testing.T) {
	if got := SignatureBlock("Bob\n"); got != "-- \nBob\n" {
		t.Errorf("SignatureBlock = %q", got)
	}
	if got := SignatureBlock(" \n"); got != "" {
		t.Errorf("a blank signature has a block: %q", got)
	}
}

func TestHasSignatureDelimiter(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"hello\n-- \nBob\n", true},
		{"hello\n--\nBob\n", true},
		{"hello\r\n-- \r\nBob\r\n", true},
		{"hello\n> -- \n> Alice\n", false},
		{"hello\n--- \n", false},
		{"a -- b\n", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := HasSignatureDelimiter(tt.text); got != tt.want {
			t.Errorf("HasSignatureDelimiter(%q) = %v, want %v", tt.text, got, tt.want)
		}
		// A text that has a delimiter is never signed again.
		if signed := SignBody(tt.text, "Bob", ""); (signed == tt.text) != tt.want {
			t.Errorf("SignBody(%q) = %q; signed again: %v", tt.text, signed, signed != tt.text)
		}
	}
}

func TestSignBody(t *testing.T) {
	quote := "On Mon, Alice wrote:\n> hi\n> -- \n> Alice\n"
	tests := []struct {
		name, text, sig, below, want string
	}{
		{"new, blank", "", "Bob", "", "\n\n-- \nBob\n"},
		{"new, with text", "Hello", "Bob", "", "Hello\n\n-- \nBob\n"},
		{"new, text ending in newlines", "Hello\n\n", "Bob\n", "", "Hello\n\n-- \nBob\n"},
		{"no signature leaves the text alone", "Hello", "", "", "Hello"},
		{"signature above the quote", "Thanks", "Bob", quote, "Thanks\n\n-- \nBob\n\n" + quote},
		{"blank reply", "", "Bob", quote, "\n\n-- \nBob\n\n" + quote},
		{"blank reply without signature", "", "", quote, "\n" + quote},
		{"reply without signature", "Thanks", "", quote, "Thanks\n\n" + quote},
		{"already signed is not signed again", "Hi\n\n-- \nOld\n", "Bob", "", "Hi\n\n-- \nOld\n"},
		{"editor-stripped delimiter counts", "Hi\n\n--\nOld\n", "Bob", "", "Hi\n\n--\nOld\n"},
		{"quoted delimiter is not the user's", "", "Bob", quote, "\n\n-- \nBob\n\n" + quote},
		{"non-ascii", "שלום", "רועי", "", "שלום\n\n-- \nרועי\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SignBody(tt.text, tt.sig, tt.below); got != tt.want {
				t.Errorf("SignBody(%q, %q, %q) =\n%q\nwant\n%q", tt.text, tt.sig, tt.below, got, tt.want)
			}
		})
	}
}

func TestReplaceSignature(t *testing.T) {
	quote := "On Mon, Alice wrote:\n> hi\n"
	tests := []struct {
		name, body, oldSig, newSig, want string
	}{
		{"swapped", "Hi\n\n-- \nWork\n", "Work", "Home", "Hi\n\n-- \nHome\n"},
		{"swapped above a quote", "Hi\n\n-- \nWork\n\n" + quote, "Work", "Home", "Hi\n\n-- \nHome\n\n" + quote},
		{"blank body", "\n\n-- \nWork\n", "Work", "Home", "\n\n-- \nHome\n"},
		{"removed when the new account has none", "Hi\n\n-- \nWork\n\n" + quote, "Work", "", "Hi\n\n" + quote},
		{"removed at the end", "Hi\n\n-- \nWork\n", "Work", "", "Hi\n"},
		{"edited signature is left", "Hi\n\n-- \nWork, edited\n", "Work", "Home", "Hi\n\n-- \nWork, edited\n"},
		{"twice is ambiguous", "-- \nWork\nfwd\n-- \nWork\n", "Work", "Home", "-- \nWork\nfwd\n-- \nWork\n"},
		{"old account had none", "Hi\n", "", "Home", "Hi\n"},
		{"same signature", "Hi\n\n-- \nWork\n", "Work", "Work\n", "Hi\n\n-- \nWork\n"},
		{"prefix of a longer line is not the block", "Hi\n\n-- \nWorkshop\n", "Work", "Home", "Hi\n\n-- \nWorkshop\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReplaceSignature(tt.body, tt.oldSig, tt.newSig); got != tt.want {
				t.Errorf("ReplaceSignature =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
