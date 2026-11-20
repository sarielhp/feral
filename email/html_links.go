package email

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// w3m's display_link_number prints "[n]" before each anchor and lists the
// targets under "References:", but never says where an anchor ends. A sign-in
// button therefore came back as the plain text "[1]Sign in", and the only
// clickable thing in the message was a list of look-alike tracking URLs at the
// bottom. markAnchorEnds puts an end marker before every </a> before the HTML
// reaches w3m, and labelLinks turns what comes back into Markdown reference
// links -- "[Sign in][1]" in the body and `[1]: https://... "Sign in"` in the
// list -- which EmbedOSC8Hyperlinks draws as a hyperlink over the label itself.

const referencesHeading = "\n\nReferences:\n\n"

var (
	anchorCloseRegex = regexp.MustCompile(`(?i)</a\s*>`)
	// referenceLineRegex matches one line of w3m's References list.
	referenceLineRegex = regexp.MustCompile(`^\[(\d+)\] (\S+)$`)
)

// markAnchorEnds returns source with a marker before every closing anchor tag,
// and the marker, a word that occurs nowhere else in source.
func markAnchorEnds(source string) (string, string) {
	marker := "mlnkend"
	lower := strings.ToLower(source)
	for strings.Contains(lower, marker) {
		marker += "z"
	}
	return anchorCloseRegex.ReplaceAllString(source, marker+"$0"), marker
}

// labelLinks rewrites a w3m dump whose anchors carry the end marker: each
// "[n]label<marker>" becomes "[label][n]", and the References entry for n
// gains the label as its title. Markers left over -- an anchor without a
// target, or one w3m did not number -- are dropped. w3m stands a link whose
// text is an image placeholder on its own, so an unlabeled anchor is named
// after its target's host.
func labelLinks(text, marker string) string {
	body, refs, found := strings.Cut(text, referencesHeading)
	if !found {
		return strings.ReplaceAll(text, marker, "")
	}
	targets := referenceTargets(refs)
	labels := map[string]string{}
	// The label may hold a bracket, as w3m's "[x]" for an image called x.png,
	// but not another numbered anchor: a "[2]" citation ahead of the anchor
	// must not be taken for its start.
	anchorRegex := regexp.MustCompile(`\[(\d+)\]((?:[^\[]|\[[^0-9\[]){0,300}?)` + regexp.QuoteMeta(marker))
	body = anchorRegex.ReplaceAllStringFunc(body, func(anchor string) string {
		parts := anchorRegex.FindStringSubmatch(anchor)
		target, ok := targets[parts[1]]
		if !ok {
			return anchor
		}
		labels[parts[1]] = linkLabel(parts[2], target)
		return "[" + labels[parts[1]] + "][" + parts[1] + "]"
	})
	body = strings.ReplaceAll(body, marker, "")
	return body + referencesHeading + titleReferences(refs, labels)
}

// referenceTargets maps each number in a References list to its target.
func referenceTargets(refs string) map[string]string {
	targets := map[string]string{}
	for line := range strings.SplitSeq(refs, "\n") {
		if m := referenceLineRegex.FindStringSubmatch(line); m != nil {
			targets[m[1]] = m[2]
		}
	}
	return targets
}

// titleReferences rewrites each "[n] target" line as a Markdown reference
// definition, `[n]: target "label"`, leaving out the title of a link that
// got no label.
func titleReferences(refs string, labels map[string]string) string {
	lines := strings.Split(refs, "\n")
	for i, line := range lines {
		m := referenceLineRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		lines[i] = "[" + m[1] + "]: " + m[2]
		if label, ok := labels[m[1]]; ok {
			lines[i] += ` "` + label + `"`
		}
	}
	return strings.Join(lines, "\n")
}

// linkLabel is the anchor text as one line, without the characters that
// would end the reference link or its title early, or the target's host when
// the text has no letter or digit to read. w3m stands "[x]" for an image
// called x.png with no alt text, and that name is the label.
func linkLabel(text, target string) string {
	label := strings.Join(strings.Fields(text), " ")
	if inner := strings.TrimSuffix(strings.TrimPrefix(label, "["), "]"); len(inner) == len(label)-2 && !strings.ContainsAny(inner, "[]") {
		label = inner
	}
	label = strings.NewReplacer("[", "(", "]", ")", `"`, "'").Replace(label)
	readable := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }
	if strings.IndexFunc(label, readable) >= 0 {
		return label
	}
	return linkHost(target)
}

// linkHost names a link by where it goes, seen through any urldefense wrapper.
func linkHost(target string) string {
	clean, _ := DecodeURLDefense(target)
	u, err := url.Parse(clean)
	if err != nil || u.Hostname() == "" {
		return "link"
	}
	return u.Hostname()
}
