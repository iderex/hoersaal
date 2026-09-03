// SPDX-FileCopyrightText: 2026 Nils Lehnen
// SPDX-License-Identifier: AGPL-3.0-or-later

package provision

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/iderex/hoersaal/internal/config"
)

func listed(t *testing.T, machines ...string) *Listed {
	t.Helper()
	l, err := NewListed(machines)
	if err != nil {
		t.Fatalf("NewListed(%v): %v", machines, err)
	}
	return l
}

func TestTheFixedPoolDriverRefusesEveryAskAndHoldsNothing(t *testing.T) {
	var d Driver = None{}
	for range 3 {
		if _, err := d.Ask(Size{}); !errors.Is(err, ErrNoCapacity) {
			t.Fatalf("Ask = %v, want %v: a fixed pool has no more at any price", err, ErrNoCapacity)
		}
	}
	held, err := d.Exists()
	if err != nil || len(held) != 0 {
		t.Fatalf("Exists = %v, %v; want nothing held", held, err)
	}
	if err := d.Remove(Handle{Machine: "anything"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Remove = %v, want %v", err, ErrUnknown)
	}
}

func TestAListedDriverHandsOutMachinesInTheOperatorsOrderAndThenRefuses(t *testing.T) {
	l := listed(t, "unit-b.example.org", "unit-a.example.org", "unit-c.example.org")
	want := []string{"unit-b.example.org", "unit-a.example.org", "unit-c.example.org"}
	for i, m := range want {
		h, err := l.Ask(Size{Egress: 1_000_000_000})
		if err != nil {
			t.Fatalf("Ask %d: %v", i, err)
		}
		if h.Machine != m || h.Unit.String() != m {
			t.Fatalf("Ask %d handed out %+v, want machine and unit %q", i, h, m)
		}
	}
	if _, err := l.Ask(Size{}); !errors.Is(err, ErrNoCapacity) {
		t.Fatalf("a fourth Ask over three machines = %v, want %v", err, ErrNoCapacity)
	}

	held, err := l.Exists()
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if len(held) != 3 {
		t.Fatalf("Exists reports %d machine(s), want 3", len(held))
	}
	for i, m := range held {
		if m.Handle.Machine != want[i] || !m.Exists {
			t.Errorf("Exists row %d is %+v, want %q and existing", i, m, want[i])
		}
	}
}

func TestRemovingAMachineHandsItOutAgainAndRemovingItTwiceIsRefused(t *testing.T) {
	l := listed(t, "unit-a.example.org", "unit-b.example.org")
	first, _ := l.Ask(Size{})
	second, _ := l.Ask(Size{})
	if err := l.Remove(first); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := l.Remove(first); !errors.Is(err, ErrUnknown) {
		t.Fatalf("a second Remove of the same handle = %v, want %v", err, ErrUnknown)
	}
	held, _ := l.Exists()
	if len(held) != 1 || held[0].Handle != second {
		t.Fatalf("after one Remove, Exists = %+v, want only %+v", held, second)
	}
	again, err := l.Ask(Size{})
	if err != nil {
		t.Fatalf("Ask after Remove: %v", err)
	}
	if again != first {
		t.Fatalf("Ask after Remove handed out %+v, want the machine given back, %+v", again, first)
	}
}

func TestRemovingWhatWasNeverHandedOutIsRefused(t *testing.T) {
	l := listed(t, "unit-a.example.org")
	if err := l.Remove(Handle{Machine: "unit-a.example.org"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Remove of a listed machine not yet handed out = %v, want %v", err, ErrUnknown)
	}
	if err := l.Remove(Handle{Machine: "unit-z.example.org"}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Remove of a machine never listed = %v, want %v", err, ErrUnknown)
	}
}

func TestAListedDriverRefusesAListItCannotHandOutFrom(t *testing.T) {
	cases := []struct {
		name     string
		machines []string
		want     error
	}{
		{"nothing listed", nil, ErrNoMachines},
		{"an empty name", []string{"unit-a.example.org", " "}, ErrEmptyMachine},
		{"a name listed twice", []string{"unit-a.example.org", "unit-a.example.org"}, ErrDuplicate},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewListed(c.machines); !errors.Is(err, c.want) {
				t.Fatalf("NewListed(%v) = %v, want %v", c.machines, err, c.want)
			}
		})
	}
}

func TestTheSizeDoesNotChangeWhichMachineAListedDriverHandsOut(t *testing.T) {
	small := listed(t, "unit-a.example.org", "unit-b.example.org")
	large := listed(t, "unit-a.example.org", "unit-b.example.org")
	a, _ := small.Ask(Size{Egress: 1})
	b, _ := large.Ask(Size{Egress: 1 << 40})
	if a != b {
		t.Fatalf("a size of 1 handed out %+v and a size of 2^40 handed out %+v; a listed machine is what it is", a, b)
	}
}

func TestTwoCallersAskingAtOnceNeverShareAMachine(t *testing.T) {
	names := []string{"unit-a.example.org", "unit-b.example.org", "unit-c.example.org"}
	l := listed(t, names...)
	const callers = 8

	var mu sync.Mutex
	var wg sync.WaitGroup
	got := map[string]int{}
	refused := 0
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := l.Ask(Size{})
			mu.Lock()
			defer mu.Unlock()
			if errors.Is(err, ErrNoCapacity) {
				refused++
				return
			}
			if err != nil {
				t.Errorf("Ask: %v", err)
				return
			}
			got[h.Machine]++
		}()
	}
	wg.Wait()

	if len(got) != len(names) || refused != callers-len(names) {
		t.Fatalf("%d machine(s) handed out and %d caller(s) refused, want %d and %d", len(got), refused, len(names), callers-len(names))
	}
	for m, n := range got {
		if n != 1 {
			t.Errorf("%s was handed out %d times", m, n)
		}
	}
}

func TestOpenAnswersWithTheDriverTheConfigurationNames(t *testing.T) {
	none, err := config.Load(strings.NewReader(`{"provisioner.driver": "none"}`))
	if err != nil {
		t.Fatalf("loading the fixed-pool configuration: %v", err)
	}
	d, err := Open(none)
	if err != nil {
		t.Fatalf("Open(none): %v", err)
	}
	if _, ok := d.(None); !ok {
		t.Fatalf("Open(none) = %T, want None", d)
	}

	over, err := config.Load(strings.NewReader(`{"provisioner.driver": "listed", "provisioner.machines": ["unit-a.example.org", "unit-b.example.org"]}`))
	if err != nil {
		t.Fatalf("loading the listed configuration: %v", err)
	}
	d, err = Open(over)
	if err != nil {
		t.Fatalf("Open(listed): %v", err)
	}
	l, ok := d.(*Listed)
	if !ok {
		t.Fatalf("Open(listed) = %T, want *Listed", d)
	}
	h, err := l.Ask(Size{})
	if err != nil || h.Machine != "unit-a.example.org" {
		t.Fatalf("the listed driver Open built hands out %+v, %v; want the first listed machine", h, err)
	}

	// A name the configuration would never accept, handed in directly, so the
	// refusal here is shown to exist rather than trusted to the loader.
	if _, err := Open(config.Settings{ProvisionerDriver: config.Driver("hyperscaler")}); !errors.Is(err, ErrUnknownDriver) {
		t.Fatalf("Open of a driver nobody built = %v, want %v", err, ErrUnknownDriver)
	}
}

// TestEveryDriverNameTheConfigurationAcceptsIsBuiltHere is the two lists held
// together: a name the loader accepts and Open refuses would reach the loop as
// an error at startup rather than as a driver, one milestone after somebody
// added it to the wrong list.
func TestEveryDriverNameTheConfigurationAcceptsIsBuiltHere(t *testing.T) {
	for _, name := range config.Drivers() {
		s := config.Settings{ProvisionerDriver: name}
		if name == config.DriverListed {
			s.ProvisionerMachines = []string{"unit-a.example.org"}
		}
		if _, err := Open(s); err != nil {
			t.Errorf("the configuration accepts %q and Open refuses it: %v", name, err)
		}
	}
}
