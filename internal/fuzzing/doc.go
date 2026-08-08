// Package fuzzing holds the fuzz targets for the surfaces a stranger can reach,
// and nothing else.
//
// It answers issue #94. Each target is an entry point taking bytes and nothing
// else, so what the fuzzer varies is exactly what an unauthenticated stranger
// varies, and a target that needed a constructed value would be measuring
// something the network cannot produce.
//
// The targets live here rather than beside each package for two reasons. The
// corpus is one thing to carry, and issue #94 asks where it is to be recorded,
// which is a poor thing to have to answer once per package. And a target in
// this package reaches its subject through the exported surface only, which is
// the surface a stranger reaches; a target inside the package under test can
// call what the network cannot.
//
// # What is covered
//
//   - FuzzSignallingEnvelope, over wire.Decode. This is the first thing an
//     unauthenticated stranger touches. The package's own comment names this
//     issue as entering there rather than through a second door, and this is
//     that entry.
//
//   - FuzzRoomCredentialToken, over roomcred.Verifier.Verify. The bytes are the
//     token exactly as a stranger sends it.
//
//   - FuzzRoomCredentialPayload, over the same Verify, with the bytes signed
//     with the verifier's own key first. This one is NOT a stranger surface and
//     is here with that written down. Verify checks the signature before it
//     reads anything inside the credential, so a target handing it unsigned
//     bytes never reaches the claims decoder at all, however long it runs. What
//     this target measures is the decoder behind the signature, which is
//     reached by a credential this installation minted and by a forger who has
//     the key. Reporting it as a stranger surface would be the more comfortable
//     sentence and it would be false.
//
// # What is not covered
//
// Issue #94 names four surfaces. Two of them are not in this tree: the
// admission handshake, which is issue #35, and the part of the media path this
// project owns rather than inherits, which arrives with the adapter on issue
// #43. Neither has an entry point to hand bytes to, so neither has a target
// here, and this sentence is what stops a green fuzz run being read as a run
// over all four.
//
// The Sender in internal/wire is also outside this package. It writes what the
// control plane sends and reads nothing from the far side, so the bytes
// crossing it are ours rather than a stranger's.
//
// # The corpus
//
// Two places, and they are different kinds of thing.
//
// The seeds and every input that ever failed are tracked, under
// testdata/fuzz/<target>/. Go runs those files on an ordinary `go test` with no
// -fuzz flag, so an input that once failed goes on being asked on every run
// afterwards, which is what issue #94 asks of a crash: a fixture and a test
// that fails before the fix and is kept after it.
//
// One tracked file needs its provenance said rather than guessed at.
// testdata/fuzz/FuzzSignallingEnvelope/0e35a6d58db40c57 is a type of sixty-five
// bytes, one over the envelope's maximum. It was not produced by a defect in
// this tree. The length check in wire.Decode was deliberately widened by one,
// the fuzzer was run against it, and it wrote that input; the check was then
// put back and the input kept. So it is a boundary case that is worth asking on
// every run, and it is not the record of a fault that ever landed.
//
// The generated corpus, everything the fuzzer found interesting and did not
// fail on, lives in the build cache under $GOCACHE/fuzz and is carried between
// scheduled runs by the cache in .github/workflows/fuzz.yml. It is not tracked,
// because it is machine-derived, it grows without bound, and nothing reads it
// but the fuzzer. A run that starts from nothing re-derives the shallow cases
// every time and never reaches the deep ones, which is the whole reason it is
// carried at all.
package fuzzing
