// SPDX-FileCopyrightText: 2026 iderex
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package config reads the one configuration this service has, refuses a key
// that is not on the fixed list, and answers issue #82.
//
// The list is not decided here. docs/decisions/what-an-operator-may-set.md
// fixes it in four groups and gives the reason for each, and this package is
// that document's list transcribed into keys plus the refusals it implies. A
// reader who disagrees with a key is sent to that document rather than to this
// file, and a key added here without a change there is the drift the check in
// internal/invariant exists to catch.
//
// # Refusing is the part that matters
//
// A loader that ignores a key it does not recognise turns a typo into a silent
// default, and the operator spends an afternoon wondering why the setting they
// wrote had no effect. Every key the file carries is compared against the list,
// and an unknown one stops the load and names itself. All of them are named
// rather than the first, because an operator with three typos should learn
// about three of them in one run.
//
// Validation happens once. Load answers with a Settings whose every field has
// already been judged, so nothing downstream checks a value a second time and
// nothing downstream has to decide what to do when it is wrong.
//
// # The format, and what it costs
//
// JSON, decoded by encoding/json. It adds no language, no runtime and no
// dependency: the graph this module declares is empty, and
// docs/decisions/control-plane-state.md has already named the store driver as
// the first entry in it, so a configuration format is not the change that
// should spend that. It carries a refusal natively, because an object member is
// a key this package can compare against a list and name back. And it is
// testable by the suite that exists, since a configuration is a string and a
// string is an io.Reader, so no case here needs a file, a process or a
// temporary directory.
//
// What it costs, stated rather than left to be discovered by the first operator
// who tries. JSON holds no comments, so an operator cannot write down why they
// set a number, and this is the file where that is most worth writing. Its
// grammar is strict in ways a hand-edited file meets: a trailing comma after the
// last member is refused, and so is a bare word where a string is meant. Both
// produce a refusal naming the position rather than a silent default, which is
// the direction this issue asks for, and neither is pleasant.
//
// # The one key with no working default
//
// Issue #82 asks that every setting have a default that works and that the
// service start with an empty configuration file. It also says, in its own
// body, that a setting with no working default is a design problem reported on
// that issue rather than a required field added to the file.
//
// The certificate is that setting, and the reason is already recorded on the
// issue. What this package does with it is hold it as absent rather than invent
// a value: the default is the empty string, an empty configuration file loads,
// and nothing here pretends the resulting deployment can serve HTTPS. That is a
// negative and it stays one. The second condition of issue #82 is not met by
// this package and this package does not read as though it were.
//
// The neighbouring question the transcription raised, and which is not the one
// already on the issue: the document names "the certificate it presents" as one
// thing, so there is one key here and no separate key for the private material
// behind it. Whether that is one setting or two is part of the same certificate
// question and is not answered by choosing a key name.
//
// # What this package does not do
//
// It does not open a file. Load takes a reader, and the open is wiring in
// cmd/hoersaal, which is where docs/repository-layout.md puts reading
// configuration. It does not listen, connect, or construct anything the
// settings describe, so a value being valid here is not a claim that the
// resource it names exists. And it holds no secret: nothing on the list is one
// today, and the day one arrives it is internal/secret's type rather than a
// string here.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// The keys. Each is a member name in the configuration object, and each carries
// the group of docs/decisions/what-an-operator-may-set.md it comes from in the
// table below. They are stable strings rather than positions, because a key is
// what an operator writes and what a refusal names back.
const (
	KeyListenAddress       = "listen.address"
	KeyListenPort          = "listen.port"
	KeyListenCertificate   = "listen.certificate"
	KeyStorePath           = "store.path"
	KeyUnitEgressCeiling   = "unit.egress-ceiling"
	KeyPoolMaximum         = "pool.maximum"
	KeyPoolMinimum         = "pool.minimum"
	KeyProvisionerDriver   = "provisioner.driver"
	KeyProvisionerMachines = "provisioner.machines"
)

// The groups, named as the document names them. They are carried per key so
// that a key can be traced to the paragraph that admitted it without opening
// the document, and so that a fifth group is visible as a fifth group rather
// than as one more line in a list.
const (
	GroupListen      = "where the service listens"
	GroupStore       = "where the store is"
	GroupHave        = "how much they have"
	GroupProvisioner = "where the machines come from"
)

// Prefixes is the set of member-name prefixes the four groups above use. It is
// five for four groups, because "how much they have" is three numbers about two
// different things: one about a unit and two about the pool.
//
// It exists for the check in internal/invariant, which needs to know what a
// configuration key looks like before it can say a string is one that is not on
// the list. That bounds the check and the bound is stated where the check is.
func Prefixes() []string {
	return []string{"listen.", "pool.", "provisioner.", "store.", "unit."}
}

// The errors. A caller that wants to tell an unreadable file from a key nobody
// declared from a value out of range can, and the three are different problems
// for an operator: the first is a syntax mistake, the second is usually a typo,
// and the third is a number they meant.
var (
	// ErrUnreadable is a configuration that is not an object.
	ErrUnreadable = errors.New("the configuration is not a JSON object")

	// ErrUnknownKey is a key that is not on the fixed list.
	ErrUnknownKey = errors.New("not a key this software has")

	// ErrValue is a key on the list carrying something it cannot hold.
	ErrValue = errors.New("not a value this key can hold")
)

// TheList names the document this package transcribes. It is in the text of
// every refusal, because the answer to "why can I not set that" is a paragraph
// in a file rather than a sentence in an error.
const TheList = "docs/decisions/what-an-operator-may-set.md"

// A Driver names the provisioning driver the pool may use. The set is a set
// this software has rather than a name an operator invents, because a driver is
// code that has to be here to be used.
type Driver string

// DriverNone is the fixed-pool case: the pool is the units that registered and
// asking for another one always fails. It is the default because no provider
// driver ships in the box, which is the answer to entry 3 on issue #1 that
// TheList records.
const DriverNone Driver = "none"

// drivers is every driver this software has. It holds one entry today and issue
// #63 is what adds to it; a name outside it is refused rather than carried to
// whatever would have dialled it.
var drivers = []Driver{DriverNone}

// Settings is one validated configuration. Every field has been judged by the
// time a caller holds one of these, so nothing downstream checks a value again
// and nothing downstream has to decide what to do when it is wrong.
type Settings struct {
	// ListenAddress is the interface the HTTPS listener binds. Empty is every
	// interface.
	ListenAddress string

	// ListenPort is the port that listener binds.
	ListenPort int

	// ListenCertificate is the certificate that listener presents. Empty means
	// none was given, which is the case the package comment records as the one
	// with no working default.
	ListenCertificate string

	// StorePath is the file docs/decisions/control-plane-state.md asks the
	// operator to be able to copy.
	StorePath string

	// UnitEgressCeiling is what the operator pays for on a unit's uplink, in
	// bits per second. Zero is the operator stating nothing, in which case the
	// egress denominator is what the machine reports and nothing else.
	UnitEgressCeiling int64

	// PoolMaximum is the largest number of units the pool may grow to, which is
	// the hard ceiling on cost.
	PoolMaximum int

	// PoolMinimum is the smallest number it keeps. A deployment that scales to
	// zero cannot admit the person who arrives first.
	PoolMinimum int

	// ProvisionerDriver is which driver may be used.
	ProvisionerDriver Driver

	// ProvisionerMachines is the machines or the endpoint that driver may use.
	// It is also the whole of what this software may contact: issue #103 is
	// where that is asserted as a property rather than as a list of hostnames.
	ProvisionerMachines []string
}

// A setting is one entry on the list: the key an operator writes, the group of
// TheList that admitted it, and how a value for it is read.
type setting struct {
	key   string
	group string
	read  func(*Settings, json.RawMessage) error
}

// list is the whole of what this software accepts. Adding an entry is adding a
// key, and TheList's own rule is that a key is added only with a written reason
// naming the deployment and the situation in which the derived value was wrong,
// and that the change naming it cites that document.
var list = []setting{
	{KeyListenAddress, GroupListen, func(s *Settings, v json.RawMessage) error {
		return readString(v, &s.ListenAddress)
	}},
	{KeyListenPort, GroupListen, func(s *Settings, v json.RawMessage) error {
		return readInt(v, &s.ListenPort)
	}},
	{KeyListenCertificate, GroupListen, func(s *Settings, v json.RawMessage) error {
		return readString(v, &s.ListenCertificate)
	}},
	{KeyStorePath, GroupStore, func(s *Settings, v json.RawMessage) error {
		return readString(v, &s.StorePath)
	}},
	{KeyUnitEgressCeiling, GroupHave, func(s *Settings, v json.RawMessage) error {
		return readInt64(v, &s.UnitEgressCeiling)
	}},
	{KeyPoolMaximum, GroupHave, func(s *Settings, v json.RawMessage) error {
		return readInt(v, &s.PoolMaximum)
	}},
	{KeyPoolMinimum, GroupHave, func(s *Settings, v json.RawMessage) error {
		return readInt(v, &s.PoolMinimum)
	}},
	{KeyProvisionerDriver, GroupProvisioner, func(s *Settings, v json.RawMessage) error {
		var name string
		if err := readString(v, &name); err != nil {
			return err
		}
		s.ProvisionerDriver = Driver(name)
		return nil
	}},
	{KeyProvisionerMachines, GroupProvisioner, func(s *Settings, v json.RawMessage) error {
		return readStrings(v, &s.ProvisionerMachines)
	}},
}

// Keys is every key this software accepts, sorted, as a fresh slice. It is a
// function rather than a variable so that no caller can add to the list from
// outside the table above, which is where the list is argued.
func Keys() []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.key)
	}
	sort.Strings(out)
	return out
}

// GroupOf is the group of TheList a key comes from, or the empty string for a
// key that is not on the list.
func GroupOf(key string) string {
	for _, s := range list {
		if s.key == key {
			return s.group
		}
	}
	return ""
}

// Defaults is the configuration of a deployment whose file is empty.
//
// Every value here is a choice rather than something read off the machine, and
// each one is argued at the field it sets or in the paragraph below, because a
// default nobody argued for is the knob TheList exists against arriving through
// the back door.
//
// The port is not 443. Every test and every run of this software is required to
// work with no privilege beyond an ordinary user account, which CONTRIBUTING.md
// calls a birth requirement, and the low ports are the ones that ask for more
// than that on the systems this is meant to run on. 8443 is the conventional
// unprivileged answer and it is a default rather than a recommendation.
//
// The address is every interface rather than the loopback one. A default that
// bound nothing reachable would make "the service starts with an empty
// configuration file" true in form and false in substance, which is the shape
// issue #82 refuses elsewhere in its own words. What decides exposure is then
// the operator's network rather than a value this software guessed, and TheList
// says in the same paragraph that there is no interface it could derive that
// would be right.
//
// The pool is one unit, floor and ceiling together, which is the fixed-pool
// case DriverNone names. That is the only default that spends no money the
// operator did not ask for, and it is coherent with the driver default: with no
// provider driver in the box there is nothing for a larger ceiling to buy.
func Defaults() Settings {
	return Settings{
		ListenAddress:     "",
		ListenPort:        8443,
		ListenCertificate: "",
		StorePath:         "hoersaal.db",
		UnitEgressCeiling: 0,
		PoolMaximum:       1,
		PoolMinimum:       1,
		ProvisionerDriver: DriverNone,
	}
}

// Load reads one configuration and answers with a validated Settings or with
// the reason it refused. It is called once, at startup, before anything the
// settings describe is constructed.
//
// An empty reader is an empty configuration and is not an error: it is the case
// issue #82's second condition asks about, and it answers with Defaults.
func Load(r io.Reader) (Settings, error) {
	s := Defaults()

	raw, err := object(r)
	if err != nil {
		return Settings{}, err
	}

	if err := refuseUnknown(raw); err != nil {
		return Settings{}, err
	}

	for _, entry := range list {
		v, ok := raw[entry.key]
		if !ok {
			continue
		}
		if err := entry.read(&s, v); err != nil {
			return Settings{}, fmt.Errorf("%s: %w", entry.key, err)
		}
	}

	if err := s.validate(); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// object decodes the whole reader into members without judging any of them. A
// reader holding nothing, or holding only space, is an empty set of members
// rather than a syntax error, because a file an operator has created and not
// yet written into is the starting state this has to accept.
func object(r io.Reader) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(r)

	raw := map[string]json.RawMessage{}
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}

	// A second value after the object is refused rather than ignored. Two
	// objects in one file is somebody who pasted a configuration under another
	// one, and taking the first silently is the shape this whole package is
	// written against.
	if dec.More() {
		return nil, fmt.Errorf("%w: it carries something after the object it opened with", ErrUnreadable)
	}
	return raw, nil
}

// refuseUnknown names every member that is not a key, rather than the first.
// The names are sorted so that one file produces one message however the
// members were ordered in it.
func refuseUnknown(raw map[string]json.RawMessage) error {
	known := map[string]bool{}
	for _, s := range list {
		known[s.key] = true
	}

	var unknown []string
	for key := range raw {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)

	quoted := make([]string, 0, len(unknown))
	for _, key := range unknown {
		quoted = append(quoted, fmt.Sprintf("%q", key))
	}
	return fmt.Errorf("%s: %w; %s fixes what an operator may set, and a key it does not name is refused rather than half applied",
		strings.Join(quoted, ", "), ErrUnknownKey, TheList)
}

// validate judges the values against each other and against what TheList and
// the documents it points at already fix. Everything it refuses stops the load,
// so nothing downstream meets a Settings that failed one of these.
func (s Settings) validate() error {
	if s.ListenPort < 1 || s.ListenPort > 65535 {
		return fmt.Errorf("%s: %w: %d is not a port", KeyListenPort, ErrValue, s.ListenPort)
	}
	if strings.TrimSpace(s.StorePath) == "" {
		return fmt.Errorf("%s: %w: the store is a file the operator can copy, which means somewhere to put it",
			KeyStorePath, ErrValue)
	}
	if s.UnitEgressCeiling < 0 {
		return fmt.Errorf("%s: %w: %d bits per second; zero states nothing and a negative states less than nothing",
			KeyUnitEgressCeiling, ErrValue, s.UnitEgressCeiling)
	}
	if s.PoolMinimum < 1 {
		return fmt.Errorf("%s: %w: %d; a deployment that scales to zero cannot admit the person who arrives first",
			KeyPoolMinimum, ErrValue, s.PoolMinimum)
	}
	if s.PoolMaximum < s.PoolMinimum {
		return fmt.Errorf("%s: %w: a ceiling of %d under a floor of %d, so the pool may not hold what it must keep",
			KeyPoolMaximum, ErrValue, s.PoolMaximum, s.PoolMinimum)
	}
	if !known(s.ProvisionerDriver) {
		return fmt.Errorf("%s: %w: %q is not a driver this software has, which today is %s",
			KeyProvisionerDriver, ErrValue, s.ProvisionerDriver, driverNames())
	}
	for _, m := range s.ProvisionerMachines {
		if strings.TrimSpace(m) == "" {
			return fmt.Errorf("%s: %w: it holds an empty entry, and an empty machine is one nothing can reach",
				KeyProvisionerMachines, ErrValue)
		}
	}
	if len(s.ProvisionerMachines) > 0 && s.ProvisionerDriver == DriverNone {
		return fmt.Errorf("%s: %w: %d machine(s) are named and %s is the driver, so nothing would ever use them",
			KeyProvisionerMachines, ErrValue, len(s.ProvisionerMachines), DriverNone)
	}
	return nil
}

func known(d Driver) bool {
	for _, candidate := range drivers {
		if candidate == d {
			return true
		}
	}
	return false
}

func driverNames() string {
	names := make([]string, 0, len(drivers))
	for _, d := range drivers {
		names = append(names, fmt.Sprintf("%q", d))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// The readers. Each refuses the value it was handed rather than coercing it, so
// a port written as a string is a refusal naming the key instead of a zero.

func readString(v json.RawMessage, into *string) error {
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return fmt.Errorf("%w: %v", ErrValue, err)
	}
	*into = s
	return nil
}

func readInt(v json.RawMessage, into *int) error {
	var n int
	if err := json.Unmarshal(v, &n); err != nil {
		return fmt.Errorf("%w: %v", ErrValue, err)
	}
	*into = n
	return nil
}

func readInt64(v json.RawMessage, into *int64) error {
	var n int64
	if err := json.Unmarshal(v, &n); err != nil {
		return fmt.Errorf("%w: %v", ErrValue, err)
	}
	*into = n
	return nil
}

func readStrings(v json.RawMessage, into *[]string) error {
	var s []string
	if err := json.Unmarshal(v, &s); err != nil {
		return fmt.Errorf("%w: %v", ErrValue, err)
	}
	*into = s
	return nil
}
