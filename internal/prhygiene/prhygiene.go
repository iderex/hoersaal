// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package prhygiene decides the things about a pull request that can be decided
// by reading it and nothing else.
//
// It answers issue #96. Three properties are judged here: every commit subject
// carries its issue reference, the change names an issue at all, and the body is
// filled in rather than left as the template. The fourth property that issue
// names, a sign-off on every non-merge commit, is deliberately absent: the DCO
// gate already refuses that and a defect refused twice is duplication rather
// than depth.
//
// Deterministic means the same input reaches the same verdict every time. There
// is no clock here, no network, no service and no model, so a contributor who
// reads a refusal can reproduce it from the pull request in front of them. That
// is what makes a red run arguable rather than something to be re-run until it
// passes.
//
// The decision is separated from the reading on purpose. Everything that talks
// to git or to the event payload is in cmd/prhygiene, so every rule below is
// exercised by a fixture in the suite rather than only by somebody opening a
// pull request that trips it.
package prhygiene

import (
	"fmt"
	"regexp"
	"strings"
)

// The rule names. They are printed with every refusal and they are what a
// contributor searches for when they want to read the rule that stopped them,
// so they are stable strings rather than positions in a list.
const (
	RuleSubjectNamesIssue = "subject-names-the-issue"
	RuleBodyNamesIssue    = "body-names-an-issue"
	RuleBodyIsFilledIn    = "body-is-filled-in"
)

// InsideAssociations is the set of authors the refusing tier applies to.
// GitHub reports the association of an author with this repository, so somebody
// who is added to it stops being exempt from that moment rather than from a
// list somebody has to maintain.
//
// COLLABORATOR is not in the set. Issue #96 asks for the outside contributor to
// have a route rather than a red check, and an occasional collaborator is in
// that position for the same reason a stranger is: the issue numbers are this
// board's convention and nothing tells them what it is before they push.
var InsideAssociations = []string{"OWNER", "MEMBER"}

// A Commit is one commit of the range a pull request proposes.
type Commit struct {
	SHA     string
	Subject string
	IsMerge bool
	IsBot   bool
}

// Input is the whole of what the check reads. A field that is not here is a
// thing no rule below can depend on, which is what keeps the verdict
// reproducible from the pull request rather than from the machine it ran on.
type Input struct {
	Body              string
	Template          string
	AuthorAssociation string
	AuthorIsBot       bool
	Commits           []Commit
}

// A Finding is one refusal, carrying enough to fix the pull request without
// opening this file.
type Finding struct {
	Rule   string
	Detail string
}

func (f Finding) String() string { return f.Rule + ": " + f.Detail }

// A Verdict is what one run decided. Skipped is separate from an empty set of
// findings because the two are different statements: one says the rules were
// applied and refused nothing, the other says they were not applied. A run that
// reported them as the same thing would let a skip read as a clean tree.
type Verdict struct {
	Findings     []Finding
	Skipped      bool
	SkipReason   string
	CommitsRead  int
	CommitsJudge int
}

// Refused is whether this verdict reds the check.
func (v Verdict) Refused() bool { return len(v.Findings) > 0 }

var (
	// The bracketed form, in the subject. Bracketed rather than bare so that a
	// version number, a port or a count in a subject is not mistaken for a
	// reference, and so the reference is visible as one to a reader of
	// git log --oneline.
	subjectRef = regexp.MustCompile(`\[#[0-9]+\]`)
	// In the body, any of the forms a person actually writes. Lenient on
	// purpose: a body that genuinely names its issue must never be refused on
	// phrasing, because that is the false refusal that teaches people to
	// ignore the check.
	bodyRef    = regexp.MustCompile(`(^|[^0-9A-Za-z_])#[0-9]+([^0-9]|$)`)
	bodyRefURL = regexp.MustCompile(`(?i)github\.com/[^/\s]+/[^/\s]+/issues/[0-9]+`)
	// A line that is only the issue reference. It satisfies the rule above and
	// it does not fill a section in, which is the difference the third rule
	// turns on.
	onlyRef = regexp.MustCompile(`^(?i:closes|refs|fixes|resolves)\s+#[0-9]+\.?$`)
	comment = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// Check reads a pull request and returns what it refuses.
func Check(in Input) Verdict {
	v := Verdict{CommitsRead: len(in.Commits)}

	if in.AuthorIsBot {
		v.Skipped = true
		v.SkipReason = "the author is a bot, and a bot cannot know the convention these rules hold a person to"
		return v
	}
	if !inside(in.AuthorAssociation) {
		v.Skipped = true
		v.SkipReason = fmt.Sprintf(
			"the author is outside this repository (author_association: %s), and nobody outside it can know an issue number before the issue exists; CONTRIBUTING.md says who supplies the reference instead",
			association(in.AuthorAssociation))
		return v
	}

	for _, c := range in.Commits {
		if c.IsMerge || c.IsBot {
			continue
		}
		v.CommitsJudge++
		if !subjectRef.MatchString(c.Subject) {
			v.Findings = append(v.Findings, Finding{
				Rule: RuleSubjectNamesIssue,
				Detail: fmt.Sprintf(
					"%s (%q) carries no issue reference in its subject; the subject is what git log, blame and bisect show, and the body is not, so the reference goes there in the form [#N]",
					short(c.SHA), c.Subject),
			})
		}
	}

	if !namesAnIssue(in.Body) {
		v.Findings = append(v.Findings, Finding{
			Rule:   RuleBodyNamesIssue,
			Detail: "the pull request body names no issue; write Closes #N where the change completes one and Refs #N where it does not",
		})
	}

	if detail, ok := stillTheTemplate(in.Body, in.Template); !ok {
		v.Findings = append(v.Findings, Finding{Rule: RuleBodyIsFilledIn, Detail: detail})
	}

	return v
}

func inside(assoc string) bool {
	for _, a := range InsideAssociations {
		if assoc == a {
			return true
		}
	}
	return false
}

func association(assoc string) string {
	if assoc == "" {
		return "NONE"
	}
	return assoc
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func namesAnIssue(body string) bool {
	stripped := comment.ReplaceAllString(body, "")
	return bodyRef.MatchString(stripped) || bodyRefURL.MatchString(stripped)
}

// stillTheTemplate answers whether the body says anything the template did not
// already say.
//
// It compares section by section against the template in the tree rather than
// against a copy held here, so editing .github/PULL_REQUEST_TEMPLATE.md is the
// whole change and this rule cannot drift away from the thing it judges.
//
// What it does not do, stated because a reader will otherwise assume it: it
// requires no particular section to be present. The template's own text invites
// deleting one section where it does not apply, so a rule demanding every
// heading would refuse the body that followed the instructions. A body that
// deletes every section and writes one sentence of its own passes this rule and
// is caught, if at all, by a reader.
func stillTheTemplate(body, template string) (string, bool) {
	if strings.TrimSpace(template) == "" {
		return "the pull request template could not be read, so this rule was not applied; failing closed rather than passing a body nothing compared", false
	}

	tmpl := map[string][]string{}
	for _, s := range sections(template) {
		tmpl[s.heading] = s.lines
	}
	for _, s := range sections(body) {
		// Anything before the first heading is the author's own preamble and
		// the template has none, so there is nothing there to have been left
		// unfilled.
		if s.heading == "" {
			continue
		}
		want, shared := tmpl[s.heading]
		if !shared {
			continue
		}
		if own(s.lines, want) {
			continue
		}
		return fmt.Sprintf(
			"the %q section carries nothing the template did not already carry; the template is the question and the body is the answer",
			s.heading), false
	}
	return "", true
}

// A section is one "## " heading and the lines under it.
type section struct {
	heading string
	lines   []string
}

// sections splits a body into its headings in the order they were written, with
// the HTML comments the template uses for its instructions removed. Order
// matters because the message a refusal prints has to be the same for the same
// body every time, and a map would name whichever empty section came out first.
// A part before the first heading is held under the empty heading, so a body
// with no headings at all is still compared.
func sections(text string) []section {
	var out []section
	at := map[string]int{}
	add := func(heading string, line string) {
		i, ok := at[heading]
		if !ok {
			out = append(out, section{heading: heading})
			i = len(out) - 1
			at[heading] = i
		}
		if line != "" {
			out[i].lines = append(out[i].lines, line)
		}
	}
	heading := ""
	add(heading, "")
	for _, line := range strings.Split(comment.ReplaceAllString(text, ""), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			heading = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			add(heading, "")
			continue
		}
		add(heading, trimmed)
	}
	return out
}

// own is whether these lines say anything the template's lines did not.
//
// A line that is only the issue reference does not count. The template carries
// "Closes #" with no number, so adding the number is the smallest possible edit
// and it is the one the second rule already asks for; letting it also discharge
// this rule would make a body consisting of the template plus one number pass
// both.
func own(lines, template []string) bool {
	seen := map[string]bool{}
	for _, l := range template {
		seen[l] = true
	}
	for _, l := range lines {
		if seen[l] || onlyRef.MatchString(l) {
			continue
		}
		return true
	}
	return false
}
