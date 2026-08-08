// Package mediaunit is the adapter between the media plane port and the
// forwarding unit this service actually runs.
//
// It is empty. Issue #43 fills it, once the unit is chosen on issue #5's
// successor work.
//
// It is the only package in this repository allowed to name a type, a constant
// or a field belonging to that unit, and the only one allowed to import its
// libraries. It is reached by cmd/hoersaal and by nothing else, so removing
// this directory leaves every other package compiling and every test passing.
// That property is what docs/repository-layout.md calls the first boundary, and
// it is the one worth running rather than reading.
package mediaunit
