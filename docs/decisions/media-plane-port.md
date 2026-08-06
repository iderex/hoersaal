# The media plane port

This is the interface the rest of the service uses to talk to a forwarding unit.
It answers issue #6. It is written before any adapter exists, because an
interface extracted after the first implementation is shaped by that
implementation and quietly assumes it.

The port names no type, constant or field belonging to any particular forwarding
unit. Which units were checked against it, and where each one does not map
cleanly, is recorded in
[media-plane-port-candidates.md](media-plane-port-candidates.md) so that this
document stays free of their vocabulary.

The port is written here as a specification and not as source, because the
language of this repository is not fixed yet and is decided on issue #15. A
specification carries the arguments, the results, the errors and the promises,
which is what the port has to fix before the fake on the next milestone can be
built against it. When the language exists, the interface is transcribed into it
and this document is what that transcription answers to.

## What is on the far side

A unit. One process that receives media from publishers and sends it to
subscribers, holding no state that the control plane cannot rebuild, and knowing
nothing about accounts, rooms as people understand them, or permissions.

Everything on the near side of the port is the control plane. It may not depend
on anything below the port, and the boundary is enforced by a test on the
architecture milestone rather than by this sentence.

## Vocabulary

These words mean the following in the port and nowhere else in the codebase are
they allowed to mean something else.

A conference is a set of participants on one unit whose media may reach each
other. It is identified by a value the control plane chooses.

A participant is one endpoint of one conference on one unit. It is identified by
a value the control plane chooses. The port has no notion of a person, and one
person joining from two devices is two participants.

A source is one stream of one kind, audio or video, offered by one publishing
participant. A participant with a camera and a microphone offers two sources. A
participant sharing a screen as well offers three.

A layer is one of the encodings of one video source that the publisher sends, in
rising order of cost. Layer zero is the cheapest encoding the publisher offers.
The port does not say how many layers exist or how they are produced; it says
that they are ordered and that a subscriber receives at most one of them per
source.

A link is a path between two units over which the media of one conference
travels, so that a conference can hold participants on both.

Load is the single number a unit reports about how close it is to its capacity.
It is defined in [capacity-signal.md](capacity-signal.md) and this document
carries it without redefining it.

## The operations

Eight operations. An operation that has meaning for one kind of unit and no
meaning for another is not in the port.

### OpenConference

Arguments: the conference identifier, and the media profile, meaning the codecs
and the layer arrangement this conference will use.

Result: nothing beyond success.

Promises on return: the unit holds the conference and will accept admissions
into it. It promises nothing about any endpoint, because none exists yet. This
is synchronous with the media plane: when it returns, the state is on the unit
and not only in a queue.

Errors: Invalid if the profile is not one the unit can serve. Conflict if the
identifier is held by a conference with a different profile. Refused if the unit
will not take another conference. Unavailable if the unit did not answer.

Opening a conference that already exists with the same profile succeeds and
changes nothing, so a control plane that lost its answer may ask again.

### CloseConference

Arguments: the conference identifier.

Result: nothing beyond success.

Promises on return: the conference is gone, every participant in it is gone,
every link that carried it is torn down on this unit's side, and the resources
are released. It does not promise that the participants have been told, because
telling them is the control plane's work over its own transport.

Errors: Unavailable. Closing a conference that is not there succeeds, because the
state the caller wanted is the state that exists.

### AdmitPublisher

Arguments: the conference identifier, the participant identifier, and the sources
this participant intends to send, each with its kind and its layer arrangement.

Result: the transport parameters the participant needs in order to connect,
opaque to the control plane, which carries them to the client and does not read
them.

Promises on return: the unit is ready to receive a connection for this
participant and will accept the sources named. It does not promise that the
participant has connected, that any packet has arrived, or that any subscriber
can see them. Those are events, and they arrive by the route in ReportFaults and
by the control plane's own signalling.

Errors: Unknown if the conference is not on this unit. Conflict if the
participant identifier is in use. Refused if the unit will not admit another
publisher. Invalid if a source names a kind or an arrangement outside the
conference profile. Unavailable.

### AdmitSubscriber

Arguments: the conference identifier and the participant identifier.

Result: the transport parameters the participant needs in order to connect, with
the same opacity as above.

Promises on return: the unit is ready to receive a connection for this
participant. The participant receives nothing until SetReception says what.

Errors: as for AdmitPublisher, without the source errors.

The two admissions are separate operations because they fail for different
reasons and cost different things, and because a room where almost everybody only
receives is the room this project is built for. A participant that both publishes
and receives holds both admissions. Whether the two admissions share one
transport or use two is the adapter's decision and the port does not promise
either.

### SetReception

Arguments: the conference identifier, the subscribing participant identifier, and
a set of entries, each naming a source and the highest layer this subscriber
would like to receive of it. A source absent from the set is not received.

Result: the set the unit accepted, in the same shape. It may differ from the set
asked for, and the difference is the answer to what the unit could do.

Promises on return: the accepted set is what the unit will try to send, from now
until the next SetReception for this subscriber. It is not a promise that the
subscriber will receive it, because what actually arrives depends on the path and
on the publisher, and a unit that promised delivery here would be lying on every
congested network.

The whole set is given every time and replaces what was there. There is no add
and no remove, because two operations that must be applied in order across a lossy
path produce a state nobody can reconstruct, and reconstructing it is exactly what
the control plane has to do after a reconnection.

Errors: Unknown for an absent conference or participant. Invalid if an entry names
a source that is not in the conference or a layer outside the profile. Unavailable.

Refused is not among them. A unit that cannot serve the set it was given answers
with the set it accepted, so that the failure is a smaller picture rather than an
error the caller has to invent a picture for.

### LinkConference

Arguments: the conference identifier, and a reference to the other unit, which
the control plane obtained from that unit and passes through without reading.

Result: a link identifier this unit will use for that path.

Promises on return: this unit will accept and send the conference's media over
the link, and participants admitted on either side may reach the other. It does
not promise that the other side is ready, so the control plane calls this on both
units and treats the conference as spanning only once both have answered.

Errors: Unknown if the conference is not here. Refused if the unit will not carry
another link. Invalid if the reference is not one this unit can use. Unavailable.

Closing a link is not an operation. A link ends when the conference ends on either
side, because a link with no conference has nothing to carry, and an operation
that could remove a link under a live conference is an operation that can split a
room in half.

### ReportCapacity

Arguments: none.

Result: the load, one number, and nothing else.

Promises on return: the number is the unit's own current view. It says nothing
about when it was computed; the pool holds the time the answer arrived and
decides for itself when an answer is too old to use. Nothing richer leaves the
unit, because the placer that reads dozens of these on every join cannot be doing
arithmetic over vectors.

Errors: Unavailable. There is no error for a unit that cannot compute its load,
because a unit that cannot say what it is holding is a unit that should not be
holding anything, and it reports that through ReportFaults instead.

### ReportFaults

Arguments: none.

Result: a stream of notices. Each notice names what was lost: a whole unit, a
conference, a participant, or a link, with the identifier of the thing named.

Promises: a notice means the thing is gone and is not coming back on its own.
Silence promises nothing at all. A unit that has died sends no notice, so the
pool decides liveness by asking rather than by waiting, and this stream is an
optimisation that shortens the delay rather than the mechanism that detects
death.

Delivery is at least once. A control plane that acts twice on one death has to be
unharmed by that, which it is, because every reaction to a death is the removal of
something.

Errors: Unavailable, meaning the stream could not be established or was broken.
Breaking is not itself a fault notice about the unit, because a broken stream and
a dead unit are different things and collapsing them retires healthy units.

## The errors

Six, and no operation invents a seventh.

Invalid, meaning the arguments are wrong and the same call will fail the same way.
Unknown, meaning the thing named is not on this unit.
Conflict, meaning the identifier is in use by something that is not what the
caller described.
Refused, meaning the unit could do this and will not, because of what it is
already holding.
Unavailable, meaning the unit did not answer, and the caller does not know whether
the operation happened.
Lost, meaning the unit answered and reported that state the caller believed in is
gone, which is the answer that follows a unit restart.

The difference between Unavailable and Lost is the difference between not knowing
and knowing the bad answer, and a control plane that treats them alike either
tears down live conferences on a slow network or leaves dead ones standing.

## What is deliberately not in the port

Recording, in any form. Whether this service can record at all is entry 2 of the
question issue, #1, and is not decided here. A port that carried a recording
operation would have decided it.

Speaker selection, layer policy and congestion response. The port carries what a
subscriber asked for and what the unit accepted. What to ask for is decided above
the port, on issues #45, #46 and #49, in one place, so that the answer does not
end up spread across every adapter.

Authentication, admission policy and moderation. The unit admits whom it is told
to admit. A unit that made its own admission decisions would be a second place
where a permission is enforced.

Anything about how the unit was started, where it runs, or how it is paid for.
That is the provisioning driver's business, on issue #63.

## How this document is checked

The port document names no forwarding unit. The names checked for are those of
the candidates in the neighbouring document:

    grep -icE 'mediasoup|livekit|janus|jitsi|videobridge|colibri|octo|galene|pion' docs/decisions/media-plane-port.md
    1

The one match is the line of this document that carries the pattern, so the
command finds itself and nothing else. The count is also the whole disclosure:
this is a grep somebody runs and not something any check refuses. Nothing in this
tree refuses a unit name appearing here, and the issue that would give the
boundary force is #98, which turns the architecture rules into tests once there
is code for them to read.
