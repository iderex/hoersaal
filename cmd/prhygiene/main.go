// SPDX-FileCopyrightText: The hoersaal contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Command prhygiene runs the deterministic pull request checks over one pull
// request and exits non-zero on any refusal. It answers issue #96.
//
// It is wiring and it decides nothing. Reading the event payload and walking
// the commit range is here; every rule is in internal/prhygiene, where the
// suite exercises each one against a fixture rather than waiting for somebody
// to open a pull request that trips it.
//
// It reads three things and no more: the event payload the run was given, the
// two ends of the commit range, and the tracked pull request template. Anything
// it cannot read is a refusal rather than a pass, because a check that judged
// nothing and a check that found nothing leave the same green tick.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/iderex/hoersaal/internal/prhygiene"
)

// templatePath is the file the body is compared against, from the repository
// root. It is the tracked template rather than a copy, so editing the template
// is the whole change.
const templatePath = ".github/PULL_REQUEST_TEMPLATE.md"

// event is the part of the pull_request payload this check reads.
type event struct {
	PullRequest struct {
		Number            int    `json:"number"`
		Body              string `json:"body"`
		AuthorAssociation string `json:"author_association"`
		User              struct {
			Type string `json:"type"`
		} `json:"user"`
	} `json:"pull_request"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "::error::%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	eventPath := os.Getenv("GITHUB_EVENT_PATH")
	base := os.Getenv("PR_BASE_SHA")
	head := os.Getenv("PR_HEAD_SHA")
	for _, needed := range []struct{ name, value string }{
		{"GITHUB_EVENT_PATH", eventPath},
		{"PR_BASE_SHA", base},
		{"PR_HEAD_SHA", head},
	} {
		if needed.value == "" {
			return fmt.Errorf("%s is empty, so this run would judge a pull request it never read; refusing rather than passing", needed.name)
		}
	}

	raw, err := os.ReadFile(eventPath)
	if err != nil {
		return fmt.Errorf("reading the event payload: %w", err)
	}
	var ev event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return fmt.Errorf("reading the event payload: %w", err)
	}
	if ev.PullRequest.Number == 0 {
		return fmt.Errorf("the event payload carries no pull request; this check runs on pull_request events and on nothing else")
	}

	template, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", templatePath, err)
	}

	commits, err := commitRange(base, head)
	if err != nil {
		return err
	}

	in := prhygiene.Input{
		Body:              ev.PullRequest.Body,
		Template:          string(template),
		AuthorAssociation: ev.PullRequest.AuthorAssociation,
		AuthorIsBot:       ev.PullRequest.User.Type == "Bot",
		Commits:           commits,
	}
	return report(ev.PullRequest.Number, prhygiene.Check(in))
}

// commitRange reads the commits a pull request proposes, newest last. The
// subject and the parent count come from git rather than from the API so that
// the range this judges is the one the merge would take.
func commitRange(base, head string) ([]prhygiene.Commit, error) {
	const sep = "\x1f"
	out, err := exec.Command("git", "log", "--reverse",
		"--format=%H"+sep+"%P"+sep+"%an <%ae>"+sep+"%s", base+".."+head).Output()
	if err != nil {
		return nil, fmt.Errorf("walking %s..%s: %w", base, head, err)
	}
	var commits []prhygiene.Commit
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, sep)
		if len(fields) != 4 {
			return nil, fmt.Errorf("a commit line came back with %d fields rather than 4: %q", len(fields), line)
		}
		commits = append(commits, prhygiene.Commit{
			SHA:     fields[0],
			Subject: fields[3],
			IsMerge: len(strings.Fields(fields[1])) > 1,
			IsBot:   strings.Contains(fields[2], "[bot]@"),
		})
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("the range %s..%s holds no commit, so there is nothing here to have judged", base, head)
	}
	return commits, nil
}

// report prints what the run did before it says whether it passed. A run that
// says only "no findings" cannot be told from one that applied no rules, and
// that distinction is the whole reason the skip below is printed rather than
// swallowed.
func report(number int, v prhygiene.Verdict) error {
	fmt.Printf("pull request #%d: %d commit(s) read, %d judged\n", number, v.CommitsRead, v.CommitsJudge)
	if v.Skipped {
		fmt.Printf("::notice::the refusing rules were not applied: %s\n", v.SkipReason)
		fmt.Println("NOT JUDGED. This is not a statement that the pull request is clean.")
		return nil
	}
	for _, f := range v.Findings {
		fmt.Printf("::error::%s\n", f)
	}
	if v.Refused() {
		return fmt.Errorf("%d hygiene refusal(s); each names the rule, and the rules are in internal/prhygiene", len(v.Findings))
	}
	fmt.Println("no findings. Every rule was applied and refused nothing.")
	return nil
}
