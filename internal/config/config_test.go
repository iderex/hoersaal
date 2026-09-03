// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Issue #82's second condition, in the half this package can answer: an empty
// configuration file loads. The half it cannot answer is the certificate, and
// the case below says so rather than leaving the green tick to imply otherwise.
func TestAnEmptyConfigurationIsTheDefaults(t *testing.T) {
	for _, in := range []string{"", "   \n\t\n", "{}"} {
		got, err := Load(strings.NewReader(in))
		if err != nil {
			t.Fatalf("%q was refused: %v", in, err)
		}
		if !reflect.DeepEqual(got, Defaults()) {
			t.Errorf("%q loaded %+v, want the defaults %+v", in, got, Defaults())
		}
	}
}

// The certificate is the setting with no working default, which is issue #82's
// own reported design problem rather than something this package solved. The
// assertion is that the default is absent and that nothing here fills it in: a
// day when this test needs changing is a day somebody invented a certificate.
func TestTheCertificateHasNoDefaultAndNothingInventsOne(t *testing.T) {
	s, err := Load(strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if s.ListenCertificate != "" {
		t.Fatalf("the certificate defaulted to %q; it has no working default and this package does not invent one",
			s.ListenCertificate)
	}
}

// The first condition. An unknown key stops the load and names itself.
func TestAnUnknownKeyStopsTheLoadAndNamesItself(t *testing.T) {
	_, err := Load(strings.NewReader(`{"listen.tls": true}`))
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("an invented key was not refused as unknown: %v", err)
	}
	if !strings.Contains(err.Error(), `"listen.tls"`) {
		t.Errorf("the refusal does not name the key: %v", err)
	}
	if !strings.Contains(err.Error(), TheList) {
		t.Errorf("the refusal does not say where the list is: %v", err)
	}
}

// A typo is the case the rule exists for, and it is one character from a key
// that is accepted. Both halves are asserted, because a loader that refused
// both would be one somebody turns off.
func TestATypoIsRefusedAndTheKeyItMissedIsNot(t *testing.T) {
	if _, err := Load(strings.NewReader(`{"pool.minimun": 2}`)); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("a one-character typo was accepted: %v", err)
	}
	got, err := Load(strings.NewReader(`{"pool.minimum": 2, "pool.maximum": 4}`))
	if err != nil {
		t.Fatalf("the key the typo missed was refused: %v", err)
	}
	if got.PoolMinimum != 2 || got.PoolMaximum != 4 {
		t.Errorf("read floor=%d ceiling=%d, want 2 and 4", got.PoolMinimum, got.PoolMaximum)
	}
}

// Every unknown key is named rather than the first one found, so an operator
// with three typos learns about three of them in one run.
func TestEveryUnknownKeyIsNamed(t *testing.T) {
	_, err := Load(strings.NewReader(`{"zebra": 1, "apple": 2, "listen.port": 9443}`))
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown keys were accepted: %v", err)
	}
	for _, key := range []string{`"apple"`, `"zebra"`} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("%s is unknown and is not named: %v", key, err)
		}
	}
	// Sorted, so that one file produces one message however it was written.
	if strings.Index(err.Error(), `"apple"`) > strings.Index(err.Error(), `"zebra"`) {
		t.Errorf("the names are not sorted, so the same file gives two messages: %v", err)
	}
}

// The third condition. Each of these is a value a key cannot hold, and each is
// a mistake an operator makes rather than one invented to make the test pass.
var refusedValues = []struct {
	why string
	in  string
}{
	{"a port outside the range", `{"listen.port": 70000}`},
	{"a port of zero, which binds whatever is free", `{"listen.port": 0}`},
	{"a port written as a string", `{"listen.port": "8443"}`},
	{"a port with a fraction", `{"listen.port": 8443.5}`},
	{"a store with nowhere to be", `{"store.path": "  "}`},
	{"a negative uplink", `{"unit.egress-ceiling": -1}`},
	{"a floor of zero, which cannot admit the first arrival", `{"pool.minimum": 0}`},
	{"a ceiling under the floor", `{"pool.minimum": 3, "pool.maximum": 2}`},
	{"a driver this software does not have", `{"provisioner.driver": "hyperscaler"}`},
	{"an empty machine nothing can reach", `{"provisioner.driver": "none", "provisioner.machines": [""]}`},
	{"machines no driver would ever use", `{"provisioner.machines": ["unit-a.example.org"]}`},
	{"a listed driver with nothing listed", `{"provisioner.driver": "listed"}`},
	{"a listed driver over an empty list", `{"provisioner.driver": "listed", "provisioner.machines": []}`},
	{"a list of machines written as one string", `{"provisioner.machines": "unit-a.example.org"}`},
}

func TestAnInvalidValueStopsTheLoad(t *testing.T) {
	for _, c := range refusedValues {
		s, err := Load(strings.NewReader(c.in))
		if err == nil {
			t.Errorf("%s: %s was accepted as %+v", c.why, c.in, s)
			continue
		}
		if !errors.Is(err, ErrValue) {
			t.Errorf("%s: refused for the wrong reason: %v", c.why, err)
		}
		if !reflect.DeepEqual(s, Settings{}) {
			t.Errorf("%s: a refusal answered with settings as well: %+v", c.why, s)
		}
	}
}

// The neighbours. Each is one change away from a case above and each is a
// configuration somebody legitimately writes, so a loader that refused one of
// these would be a loader somebody edits rather than argues with.
var acceptedValues = []struct {
	why string
	in  string
}{
	{"the lowest and highest ports", `{"listen.port": 1}`},
	{"the highest port", `{"listen.port": 65535}`},
	{"every interface, written out", `{"listen.address": ""}`},
	{"an uplink of zero states nothing", `{"unit.egress-ceiling": 0}`},
	{"a floor equal to the ceiling is a fixed pool", `{"pool.minimum": 3, "pool.maximum": 3}`},
	{"an empty machine list under the fixed-pool driver", `{"provisioner.machines": []}`},
	{"a listed driver over one machine", `{"provisioner.driver": "listed", "provisioner.machines": ["unit-a.example.org"]}`},
	{"a certificate that was given", `{"listen.certificate": "/etc/hoersaal/fullchain.pem"}`},
}

func TestTheNeighboursAreAccepted(t *testing.T) {
	for _, c := range acceptedValues {
		if _, err := Load(strings.NewReader(c.in)); err != nil {
			t.Errorf("%s: %s was refused: %v", c.why, c.in, err)
		}
	}
}

// A file that is not an object at all is refused as unreadable rather than as a
// key nobody declared, because those are different mistakes and an operator
// fixes them differently.
func TestSomethingThatIsNotAnObjectIsRefusedAsUnreadable(t *testing.T) {
	for _, in := range []string{
		`[]`,
		`"listen.port"`,
		`{"listen.port": 8443,}`,
		`{"listen.port": 8443} {"pool.minimum": 2}`,
	} {
		if _, err := Load(strings.NewReader(in)); !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s was not refused as unreadable: %v", in, err)
		}
	}
}

// The list is the subject of the fourth condition, so its own shape is
// asserted: no key appears twice, every key belongs to a group of TheList, and
// every key begins with one of the prefixes the check in internal/invariant
// reads. The last of the three is what makes that check's reach a fact rather
// than an assumption.
func TestEveryKeyIsUniqueGroupedAndInsideThePrefixes(t *testing.T) {
	if len(list) == 0 {
		t.Fatal("the list is empty, so this suite asserts nothing")
	}
	groups := map[string]bool{
		GroupListen: true, GroupStore: true, GroupHave: true, GroupProvisioner: true,
	}
	seen := map[string]bool{}
	for _, key := range Keys() {
		if seen[key] {
			t.Errorf("%s appears twice, so one of the two is unreachable", key)
		}
		seen[key] = true

		group := GroupOf(key)
		if !groups[group] {
			t.Errorf("%s carries the group %q, which is not one of the four %s names", key, group, TheList)
		}

		inside := false
		for _, p := range Prefixes() {
			if strings.HasPrefix(key, p) {
				inside = true
			}
		}
		if !inside {
			t.Errorf("%s begins with none of the prefixes, so the check that reads them cannot see it", key)
		}
	}
}

// The fourth condition of issue #82: the set of settings matches the list on
// the decisions milestone. The document is the authority and this reads it
// rather than holding a second copy, so a key added to the table above and not
// to that document reds the run, and so does a key the document names and this
// package does not accept.
//
// It reads the block under the heading the document gives it. A block rather
// than the prose, because prose cannot be compared and a comparison that had to
// guess would be one somebody deletes.
func TestTheKeysAreTheOnesTheDocumentFixes(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(TheList)))
	if err != nil {
		t.Fatalf("%s is what this package is transcribed from and it could not be read: %v", TheList, err)
	}

	fixed := keysInTheBlock(string(source))
	if len(fixed) == 0 {
		t.Fatalf("no key was found under the heading %q in %s, so this test asserts nothing", blockHeading, TheList)
	}

	if !reflect.DeepEqual(fixed, Keys()) {
		t.Errorf("%s fixes %v and this package accepts %v", TheList, fixed, Keys())
	}
}

// blockHeading is the section of TheList that carries the keys. It is a
// constant here because the test has to name where it read from when it fails,
// and a reader meeting that failure needs the heading rather than a line number
// that will have moved.
const blockHeading = "## The same list, as the keys an operator writes"

// keysInTheBlock is every indented line under blockHeading, up to the next
// heading, sorted. The document indents the block by four spaces, which is how
// it writes every command and every quoted output, so nothing new is being
// asked of it.
func keysInTheBlock(source string) []string {
	_, after, found := strings.Cut(source, blockHeading)
	if !found {
		return nil
	}
	if next, _, ok := strings.Cut(after, "\n## "); ok {
		after = next
	}

	var keys []string
	for _, line := range strings.Split(after, "\n") {
		if !strings.HasPrefix(line, "    ") {
			continue
		}
		key := strings.TrimSpace(line)
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// Keys answers sorted and answers with a fresh slice each time, so a caller
// cannot reorder or extend the list from outside the table that argues it.
func TestKeysIsSortedAndCannotBeWrittenThrough(t *testing.T) {
	first := Keys()
	for i := 1; i < len(first); i++ {
		if first[i-1] >= first[i] {
			t.Fatalf("Keys is not sorted at %d: %v", i, first)
		}
	}
	first[0] = "listen.tampered"
	if Keys()[0] == "listen.tampered" {
		t.Fatal("writing into the answer changed the list")
	}
	if GroupOf("listen.tampered") != "" {
		t.Fatal("a key that is not on the list was given a group")
	}
}

// The defaults are a configuration this loader accepts. A default set that its
// own validation refuses is a service that cannot start with an empty file, and
// the second condition would then be false in a way no other case here catches.
func TestTheDefaultsPassTheirOwnValidation(t *testing.T) {
	if err := Defaults().validate(); err != nil {
		t.Fatalf("the defaults are refused by validation: %v", err)
	}
}

// Load answers with a value rather than a pointer into anything it read, and it
// judges once. The assertion that matters for "validated once" is that a
// Settings a caller holds came through validate: there is no exported way to
// build one that did not, other than composing the struct literally, which a
// reader can see.
func TestLoadAnswersWithSomethingAlreadyJudged(t *testing.T) {
	s, err := Load(strings.NewReader(`{"pool.minimum": 2, "pool.maximum": 5, "listen.port": 9443}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.validate(); err != nil {
		t.Fatalf("Load answered with something its own validation refuses: %v", err)
	}
	if s.ListenPort != 9443 || s.PoolMinimum != 2 || s.PoolMaximum != 5 {
		t.Errorf("read %+v, want port 9443 and a pool of 2 to 5", s)
	}
}

// Two keys of the same group set two different fields, which is the case a
// table with a copy-and-paste mistake in it fails and a reader does not see.
func TestEveryKeySetsItsOwnField(t *testing.T) {
	const all = `{
		"listen.address": "10.0.0.4",
		"listen.port": 9443,
		"listen.certificate": "/etc/hoersaal/fullchain.pem",
		"store.path": "/var/lib/hoersaal/state.db",
		"unit.egress-ceiling": 1000000000,
		"pool.minimum": 2,
		"pool.maximum": 6,
		"provisioner.driver": "none"
	}`
	s, err := Load(strings.NewReader(all))
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{
		ListenAddress:     "10.0.0.4",
		ListenPort:        9443,
		ListenCertificate: "/etc/hoersaal/fullchain.pem",
		StorePath:         "/var/lib/hoersaal/state.db",
		UnitEgressCeiling: 1000000000,
		PoolMaximum:       6,
		PoolMinimum:       2,
		ProvisionerDriver: DriverNone,
	}
	if s.ListenAddress != want.ListenAddress || s.ListenPort != want.ListenPort ||
		s.ListenCertificate != want.ListenCertificate || s.StorePath != want.StorePath ||
		s.UnitEgressCeiling != want.UnitEgressCeiling || s.PoolMaximum != want.PoolMaximum ||
		s.PoolMinimum != want.PoolMinimum || s.ProvisionerDriver != want.ProvisionerDriver {
		t.Errorf("read %+v, want %+v", s, want)
	}
	if len(s.ProvisionerMachines) != 0 {
		t.Errorf("machines were invented: %v", s.ProvisionerMachines)
	}
}
