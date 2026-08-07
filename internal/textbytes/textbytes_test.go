package textbytes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTreeIsClean is the check itself. It reads the repository it lives in and
// fails on any text file carrying a carriage return.
func TestTreeIsClean(t *testing.T) {
	findings, err := CheckTree("../..")
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}

// TestTheTreeDeclaresItsTextAttributes is the other half of the same reading.
// The tree can be clean because it has no carriage return in it today and still
// be one clone away from carrying one on every line, so the declaration is
// asserted rather than assumed.
func TestTheTreeDeclaresItsTextAttributes(t *testing.T) {
	attrs, err := os.ReadFile(filepath.Join("../..", attributesFile))
	if err != nil {
		t.Fatalf("reading %s: %v", attributesFile, err)
	}
	if !strings.Contains(string(attrs), "* text=auto eol=lf") {
		t.Errorf("%s does not declare `* text=auto eol=lf`, so a checkout decides the line endings from a personal setting", attributesFile)
	}
	if _, findings := BinarySuffixes(attrs); len(findings) != 0 {
		for _, f := range findings {
			t.Errorf("%s", f)
		}
	}
}

// The rest of this file proves the rules bite. Each fixture is bytes the
// checker is asked about directly, so a rule that stopped refusing shows up
// here rather than waiting for somebody to write the mistake.
//
// The bytes are written as escapes rather than as raw literals on purpose. A
// raw carriage return in this file would be the very thing the tree check
// above refuses, so the fixture that proves the guard bites cannot be spelled
// the way the defect is spelled.

var declared = []string{".png", ".ivf"}

func TestACarriageReturnBeforeALineFeedIsRefused(t *testing.T) {
	src := []byte("package p\r\n\r\nfunc f() {}\r\n")
	findings := CheckFile("internal/pool/pool.go", src, declared)
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Line != 1 {
		t.Errorf("want the finding at line 1, got %d", findings[0].Line)
	}
	if !strings.Contains(findings[0].Detail, "did not honour the text attributes") {
		t.Errorf("the finding does not say which of the two shapes it is: %s", findings[0])
	}
	if !strings.Contains(findings[0].Detail, "(3 in the file)") {
		t.Errorf("the finding does not count what it found: %s", findings[0])
	}
}

// The near-miss. `text=auto` normalises a carriage return that is followed by a
// line feed and leaves a lone one alone, so this is the shape that survives
// every checkout on every platform and that no attribute refuses. It is also
// the one-character mistake somebody actually makes, by pasting a line out of a
// terminal or an editor that ends lines the old way.
func TestALoneCarriageReturnIsRefused(t *testing.T) {
	src := []byte("package p\n\nconst greeting = \"hello\rworld\"\n")
	findings := CheckFile("internal/pool/pool.go", src, declared)
	if len(findings) != 1 {
		t.Fatalf("want 1 finding for a lone carriage return, got %d: %v", len(findings), findings)
	}
	if findings[0].Line != 3 {
		t.Errorf("want the finding at line 3, got %d", findings[0].Line)
	}
	if !strings.Contains(findings[0].Detail, "no line feed after it") {
		t.Errorf("the finding reads as the other shape: %s", findings[0])
	}
}

func TestLineFeedsAloneAreNotRefused(t *testing.T) {
	src := []byte("package p\n\nfunc f() {}\n")
	if got := CheckFile("internal/pool/pool.go", src, declared); len(got) != 0 {
		t.Errorf("a file with line feeds only is what the rule asks for: %v", got)
	}
}

func TestADeclaredBinaryPathIsNotRead(t *testing.T) {
	src := []byte("\r\n\r\nnot really a picture\r\n")
	if got := CheckFile("internal/media/testdata/frame.png", src, declared); len(got) != 0 {
		t.Errorf("a path declared binary is stored raw and is not this guard's business: %v", got)
	}
	if got := CheckFile("internal/media/testdata/FRAME.PNG", src, declared); len(got) != 0 {
		t.Errorf("the suffix match is not case sensitive: %v", got)
	}
}

func TestAnUndeclaredSuffixIsStillRead(t *testing.T) {
	src := []byte("first\r\nsecond\r\n")
	if got := CheckFile("internal/media/testdata/frame.rtp", src, declared); len(got) != 1 {
		t.Errorf("a suffix nobody declared binary is text, and text is read: %v", got)
	}
}

func TestAZeroByteMakesAFileBinaryByContent(t *testing.T) {
	src := []byte("\x00\x01\x02\r\n\x03")
	if got := CheckFile("internal/media/testdata/frame.rtp", src, declared); len(got) != 0 {
		t.Errorf("a file holding a zero byte is binary by the rule git itself applies: %v", got)
	}
}

// The zero byte has to be inside the window git looks at. A payload that is
// text for the first eight thousand bytes is one git would normalise, so this
// guard has to judge it the same way rather than excusing it.
func TestAZeroByteBeyondTheWindowDoesNotExcuseAFile(t *testing.T) {
	src := append([]byte(strings.Repeat("a", binaryScan)+"\r\n"), 0)
	if got := CheckFile("internal/media/testdata/frame.rtp", src, declared); len(got) != 1 {
		t.Errorf("want the file judged as text, got %v", got)
	}
}

func TestTheBinaryPatternsComeFromTheAttributes(t *testing.T) {
	attrs := []byte("# a comment\n\n* text=auto eol=lf\n*.png binary\n*.ivf -text\n*.md text\n")
	suffixes, findings := BinarySuffixes(attrs)
	if len(findings) != 0 {
		t.Fatalf("nothing here is unreadable: %v", findings)
	}
	want := ".png .ivf"
	if got := strings.Join(suffixes, " "); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

// Fail closed. A binary declaration this guard cannot read is one whose files
// it would judge as text while git stored them raw, which is a guard that has
// quietly stopped covering part of the tree.
func TestABinaryPatternThisGuardCannotReadIsRefused(t *testing.T) {
	attrs := []byte("* text=auto eol=lf\ndocs/*.png binary\n")
	suffixes, findings := BinarySuffixes(attrs)
	if len(suffixes) != 0 {
		t.Errorf("a pattern that was refused must not also be used: %v", suffixes)
	}
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Line != 2 {
		t.Errorf("want the finding at line 2, got %d", findings[0].Line)
	}
}

func TestAMissingAttributesFileIsRefused(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	findings, err := CheckTree(root)
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	if len(findings) != 1 || findings[0].Path != attributesFile {
		t.Fatalf("want one finding about %s, got %v", attributesFile, findings)
	}
	if !strings.Contains(findings[0].Detail, "is missing") {
		t.Errorf("the finding does not say what is missing: %s", findings[0])
	}
}

func TestASecondAttributesFileIsRefused(t *testing.T) {
	root := t.TempDir()
	write(t, root, attributesFile, "* text=auto eol=lf\n")
	write(t, root, filepath.Join("internal", attributesFile), "* -text\n")
	findings, err := CheckTree(root)
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != "internal/"+attributesFile {
		t.Errorf("the finding names the wrong file: %s", findings[0])
	}
}

// The walk reads the dotted directories too. The workflow files are tracked
// text and a carriage return in one of them is the same defect as in the
// source, so a walk that skipped them would report a clean tree it had not read.
func TestTheWalkReadsDottedDirectories(t *testing.T) {
	root := t.TempDir()
	write(t, root, attributesFile, "* text=auto eol=lf\n")
	write(t, root, filepath.Join(".github", "workflows", "unit.yml"), "name: Unit\r\n")
	findings, err := CheckTree(root)
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != ".github/workflows/unit.yml" {
		t.Errorf("the finding names the wrong file: %s", findings[0])
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
