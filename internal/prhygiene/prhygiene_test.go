package prhygiene

import (
	"os"
	"strings"
	"testing"
)

// treeTemplate is the pull request template as the repository tracks it. Every
// case below judges against it rather than against a copy written here, so a
// change to the template moves these tests with it instead of leaving them
// asserting something the tree stopped saying.
func treeTemplate(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("../../.github/PULL_REQUEST_TEMPLATE.md")
	if err != nil {
		t.Fatalf("reading the pull request template: %v", err)
	}
	if len(src) == 0 {
		t.Fatal("the pull request template is empty, so every case below would be judged against nothing")
	}
	return string(src)
}

// good is a body that answers every section the template asks about.
const good = `## What changed

The hygiene check refuses three things about a pull request.

## What failure it prevents

A change that reaches main with no link to the issue that argued for it.

Closes #96

## What was run

    go test -count=1 ./internal/prhygiene
    ok

## The means

Go, which the tree already carries.
`

func clean(t *testing.T) Input {
	t.Helper()
	return Input{
		Body:              good,
		Template:          treeTemplate(t),
		AuthorAssociation: "OWNER",
		Commits: []Commit{
			{SHA: "1111111111", Subject: "Refuse a pull request that names no issue [#96]"},
		},
	}
}

func rules(v Verdict) []string {
	var out []string
	for _, f := range v.Findings {
		out = append(out, f.Rule)
	}
	return out
}

func refusedBy(v Verdict, rule string) bool {
	for _, f := range v.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

// TestACleanPullRequestPasses is the green case every red case below is one
// change away from. Without it a rule that refused everything would look like a
// rule that worked.
func TestACleanPullRequestPasses(t *testing.T) {
	v := Check(clean(t))
	if v.Refused() {
		t.Fatalf("a clean pull request was refused by %v", rules(v))
	}
	if v.Skipped {
		t.Fatal("a clean pull request from an owner was skipped rather than judged")
	}
	if v.CommitsJudge != 1 {
		t.Fatalf("judged %d commits, want 1; a run that judged none refuses nothing and looks the same as a clean one", v.CommitsJudge)
	}
}

// TestASubjectWithoutItsIssueIsRefused is the first condition of issue #96. The
// input differs from the clean case in the bracket and nothing else, which is
// the mistake somebody actually makes rather than one nobody would.
func TestASubjectWithoutItsIssueIsRefused(t *testing.T) {
	in := clean(t)
	in.Commits = []Commit{
		{SHA: "1111111111", Subject: "Refuse a pull request that names no issue"},
	}
	v := Check(in)
	if !refusedBy(v, RuleSubjectNamesIssue) {
		t.Fatalf("a subject carrying no issue reference was not refused; findings: %v", rules(v))
	}
	if !strings.Contains(v.Findings[0].Detail, "1111111") {
		t.Errorf("the refusal does not name the commit it is about: %q", v.Findings[0].Detail)
	}
}

// TestABareNumberInASubjectIsNotAReference is the near miss. A subject naming a
// port, a count or a version would satisfy a rule that looked for a digit
// after a hash, and the bracketed form is what tells a reference from a number.
func TestABareNumberInASubjectIsNotAReference(t *testing.T) {
	in := clean(t)
	in.Commits = []Commit{
		{SHA: "2222222222", Subject: "Hold the reconnect window at 30 seconds for #the room"},
	}
	if !refusedBy(Check(in), RuleSubjectNamesIssue) {
		t.Error("a subject with a number in it passed as one carrying an issue reference")
	}
}

// TestAMergeCommitAndABotCommitAreNotJudged keeps the rule off the two authors
// that cannot follow it. A merge subject is written by the server and a bot has
// no issue to name.
func TestAMergeCommitAndABotCommitAreNotJudged(t *testing.T) {
	in := clean(t)
	in.Commits = []Commit{
		{SHA: "3333333333", Subject: "Merge pull request #141 from iderex/ci/pr-hygiene", IsMerge: true},
		{SHA: "4444444444", Subject: "Bump an action to its next digest", IsBot: true},
		{SHA: "1111111111", Subject: "Refuse a pull request that names no issue [#96]"},
	}
	v := Check(in)
	if v.Refused() {
		t.Fatalf("a merge or a bot commit was judged: %v", rules(v))
	}
	if v.CommitsJudge != 1 {
		t.Fatalf("judged %d of 3 commits, want 1", v.CommitsJudge)
	}
}

// TestABodyThatNamesNoIssueIsRefused is the second condition. The template
// ships the word and not the number, so this is the case of somebody pushing
// the template's own line unchanged.
func TestABodyThatNamesNoIssueIsRefused(t *testing.T) {
	in := clean(t)
	in.Body = strings.Replace(good, "Closes #96", "Closes #", 1)
	if !refusedBy(Check(in), RuleBodyNamesIssue) {
		t.Error("a body naming no issue was not refused")
	}
}

// TestAnIssueURLCountsAsNamingTheIssue is the leniency the rule is written
// with. Refusing a body that genuinely links its issue, on the phrasing, is the
// false refusal that teaches a contributor to stop reading the check.
func TestAnIssueURLCountsAsNamingTheIssue(t *testing.T) {
	in := clean(t)
	in.Body = strings.Replace(good, "Closes #96",
		"See https://github.com/iderex/hoersaal/issues/96", 1)
	if refusedBy(Check(in), RuleBodyNamesIssue) {
		t.Error("a body linking its issue by URL was refused as naming none")
	}
}

// TestTheTemplateLeftAsItIsIsRefused is the third condition, at its extreme:
// the body is the template, pushed without a word added.
func TestTheTemplateLeftAsItIsIsRefused(t *testing.T) {
	in := clean(t)
	in.Body = in.Template
	if v := Check(in); !refusedBy(v, RuleBodyIsFilledIn) {
		t.Errorf("the template pushed unchanged was not refused; findings: %v", rules(v))
	}
}

// TestTheTemplatePlusOnlyTheIssueNumberIsRefused is the near miss for the same
// rule, and it is the one worth spending the effort on. Adding the number is
// the smallest edit anybody makes to the template, it discharges the second
// rule on its own, and a check that let it discharge this one too would pass a
// body that still says nothing.
func TestTheTemplatePlusOnlyTheIssueNumberIsRefused(t *testing.T) {
	in := clean(t)
	in.Body = strings.Replace(in.Template, "Closes #", "Closes #96", 1)
	v := Check(in)
	if refusedBy(v, RuleBodyNamesIssue) {
		t.Fatal("the body names its issue, so the second rule should be silent here")
	}
	if !refusedBy(v, RuleBodyIsFilledIn) {
		t.Error("the template plus an issue number passed as a filled-in body")
	}
}

// TestASectionAnsweredInTheAuthorsOwnWordsPasses is the other direction of the
// same rule. A section whose answer happens to be short is still an answer.
func TestASectionAnsweredInTheAuthorsOwnWordsPasses(t *testing.T) {
	in := clean(t)
	in.Body = "## What changed\n\nOne line.\n\n## What failure it prevents\n\nAnother.\n\nCloses #96\n"
	if refusedBy(Check(in), RuleBodyIsFilledIn) {
		t.Error("a short but genuine answer was refused as the template")
	}
}

// TestAnUnreadableTemplateFailsClosed. A template that could not be read is a
// rule that judged nothing, and reporting that as a pass is the shape this
// tree's other guards are written against.
func TestAnUnreadableTemplateFailsClosed(t *testing.T) {
	in := clean(t)
	in.Template = ""
	v := Check(in)
	if !refusedBy(v, RuleBodyIsFilledIn) {
		t.Fatal("an unread template passed the body it could not compare")
	}
}

// TestAnOutsideAuthorIsSkippedAndSaysSo is the case issue #96 asks to be
// handled rather than left red. The skip has to be visible: a run reporting no
// findings and a run reporting that it applied no rules are different
// statements, and collapsing them is how a skip comes to read as a clean tree.
func TestAnOutsideAuthorIsSkippedAndSaysSo(t *testing.T) {
	for _, assoc := range []string{"NONE", "CONTRIBUTOR", "COLLABORATOR", ""} {
		in := clean(t)
		in.AuthorAssociation = assoc
		in.Body = "no issue here"
		in.Commits = []Commit{{SHA: "5555555555", Subject: "A first contribution"}}
		v := Check(in)
		if v.Refused() {
			t.Errorf("author_association %q was refused rather than skipped: %v", assoc, rules(v))
		}
		if !v.Skipped {
			t.Errorf("author_association %q was judged as inside this repository", assoc)
		}
		if strings.TrimSpace(v.SkipReason) == "" {
			t.Errorf("author_association %q was skipped with no reason given", assoc)
		}
	}
}

// TestABotIsSkippedForItsOwnReason. The two skips are not the same statement
// and a reader of a run has to be able to tell which one happened.
func TestABotIsSkippedForItsOwnReason(t *testing.T) {
	in := clean(t)
	in.AuthorIsBot = true
	in.Body = ""
	v := Check(in)
	if !v.Skipped || v.Refused() {
		t.Fatalf("a bot was judged: skipped=%v findings=%v", v.Skipped, rules(v))
	}
	if !strings.Contains(v.SkipReason, "bot") {
		t.Errorf("the bot skip does not say it was a bot: %q", v.SkipReason)
	}
}

// TestAnInsideAuthorIsNotSkipped is what stops the two skips above from
// swallowing the whole check.
func TestAnInsideAuthorIsNotSkipped(t *testing.T) {
	for _, assoc := range InsideAssociations {
		in := clean(t)
		in.AuthorAssociation = assoc
		if Check(in).Skipped {
			t.Errorf("author_association %q was skipped", assoc)
		}
	}
}

// TestTheVerdictIsTheSameEveryTime is the deterministic part of the issue's own
// title, asserted rather than claimed. The same input is judged repeatedly and
// the findings, including the order and the wording, have to match.
func TestTheVerdictIsTheSameEveryTime(t *testing.T) {
	in := clean(t)
	in.Body = in.Template
	in.Commits = []Commit{{SHA: "6666666666", Subject: "No reference at all"}}
	first := Check(in)
	for i := 0; i < 32; i++ {
		got := Check(in)
		if len(got.Findings) != len(first.Findings) {
			t.Fatalf("run %d found %d findings, the first found %d", i, len(got.Findings), len(first.Findings))
		}
		for j := range got.Findings {
			if got.Findings[j] != first.Findings[j] {
				t.Fatalf("run %d finding %d is %v, the first run had %v", i, j, got.Findings[j], first.Findings[j])
			}
		}
	}
}

// TestTheTreesOwnTemplateIsJudgeable is the leg that stops this file passing
// over a template that has stopped having sections. Every case above compares
// against the tracked file, so a template reduced to prose would make the third
// rule vacuous while every test here stayed green.
func TestTheTreesOwnTemplateIsJudgeable(t *testing.T) {
	found := 0
	for _, s := range sections(treeTemplate(t)) {
		if s.heading != "" {
			found++
		}
	}
	if found < 3 {
		t.Fatalf("the tracked template has %d sections; the rule that a body answers it needs sections to compare", found)
	}
}
