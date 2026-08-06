# Candidate forwarding units checked against the port

The port is in [media-plane-port.md](media-plane-port.md) and names no
forwarding unit. This document is where the units are named. It answers the third
condition of issue #6: which candidates were checked, and which operations do not
map cleanly to each.

It does not choose a unit. That is issue #5.

## What was read, and what that bounds

Each statement below is about a document, at the address given, read on
2026-08-06. Nothing here was run, no unit was deployed, and no source tree was
read. So a mapping recorded as absent means it was absent from the pages named,
which is weaker than absent from the software, and where that difference matters
it is said in the row rather than left to the reader.

    mediasoup      https://mediasoup.org/documentation/v3/mediasoup/api/
    Janus VideoRoom https://janus.conf.meetecho.com/docs/videoroom.html
    LiveKit        https://docs.livekit.io/reference/server/server-apis/
                   https://docs.livekit.io/home/self-hosting/distributed/
    Jitsi Videobridge  doc/rest-colibri2.md, doc/relay.md, doc/statistics.md and
                   doc/health-checks.md in the jitsi/jitsi-videobridge repository,
                   read at the default branch through the platform API

## mediasoup

A router is the conference. `worker.createRouter` opens one and `router.close`
ends it, so OpenConference and CloseConference map directly. A router is created
on a worker, which the documentation describes as a subprocess, so the unit in
this port's sense is a process that holds several routers rather than the router
itself.

Admission maps onto two calls rather than one. A transport is created first,
`router.createWebRtcTransport`, and then `transport.produce` or
`transport.consume` attaches media to it. So AdmitPublisher is a transport
creation whose result carries the connection parameters, and the sources named in
the admission become produce calls the adapter makes when the client offers them.
AdmitSubscriber is a transport creation with no consume call yet, which is
exactly what the port says a subscriber gets before SetReception.

SetReception maps onto three calls and not one: `transport.consume` to start
receiving a source, `consumer.setPreferredLayers` to pick the layer, and
`consumer.pause` and `consumer.resume` for the sources the subscriber has
dropped. The port's whole-set replacement is therefore a difference the adapter
computes, which is work the adapter does and not a hole in the port.

LinkConference maps onto `router.pipeToRouter`, which the documentation describes
as piping a producer into another router. That is per producer rather than per
conference, so the adapter holds the link and pipes each new publisher across it.
Whether the same call reaches a router in another process on another machine is
not settled by the page read, and this is the row where the difference between
absent from the page and absent from the software matters most.

ReportCapacity has no clean mapping. `worker.getResourceUsage` reports the
subprocess resource usage, and `transport.getStats`, `producer.getStats` and
`consumer.getStats` report per object statistics. There is no single normalised
number, so the adapter computes the load itself from the inputs named in
[capacity-signal.md](capacity-signal.md).

ReportFaults was not evaluated on this route. The API page read lists methods and
not events, so whether the runtime reports a dead worker is a question this
reading does not answer.

## Janus, VideoRoom plugin

`create` and `destroy` are the conference, and both are synchronous requests,
which is what OpenConference and CloseConference ask for.

`join` and `publish`, or `joinandconfigure`, are the publisher's admission, and
`subscribe` is the subscriber's. Both are asynchronous, so the promise
AdmitPublisher makes on return has to be made by the adapter waiting for the
plugin's event rather than by the request returning.

SetReception maps onto `subscribe`, `unsubscribe` and `update` for which sources,
and onto `configure` with its `substream` and `temporal` fields for which layer.
The port asks for one operation carrying both, so the adapter splits it.

LinkConference does not map. The plugin has `rtp_forward` and `listforwarders`,
which forward a publisher's RTP to a destination. That is one publisher leaving
towards an address, not two instances holding one conference between them, and
building the second out of the first would mean the adapter reimplementing the
part of a cascade that is hardest to get right. Of the four candidates this is
the one where the port asks for something the documented interface does not
offer.

ReportCapacity does not map. The plugin documentation carries no capacity or load
figure. Janus has interfaces outside this plugin and they were not read, so this
row is about the VideoRoom page and not about Janus.

ReportFaults maps in part, through the plugin's asynchronous events, which were
not enumerated on this reading.

## LiveKit

`CreateRoom` and `DeleteRoom` are the conference, and `DeleteRoom` is documented
as forcibly disconnecting all participants, which is what CloseConference
promises.

Admission does not map. The server API read has no call that admits a
participant. A participant connects with a token and the room accepts them, so
AdmitPublisher and AdmitSubscriber become the issuing of a credential rather than
a call to the unit, and the transport parameters the port returns are a token
instead. `RemoveParticipant` exists for the reverse direction. This is the
sharpest mismatch in the set, and it is a mismatch about who initiates rather
than about what is possible.

SetReception maps onto `UpdateSubscriptions`, which the reference describes as
subscribing or unsubscribing a participant from published tracks. Layer selection
did not appear in the reference read, so which layer a subscriber receives is not
answered by this route.

LinkConference does not map. The distributed deployment page describes a node
selector choosing which node hosts a room, and names no operation that spreads one
room across two nodes.

ReportCapacity maps in shape rather than in call. Nodes report stats and a node is
eligible for a new room while its utilisation is below `sysload_limit`, so the
system carries a normalised load and a threshold over it, which is the design in
[capacity-signal.md](capacity-signal.md) arrived at independently. It is not an
operation on a unit, so the adapter would read it from where the cluster keeps it.

## Jitsi Videobridge

This is the closest fit of the four, and every operation of the port has
something on the other side.

`POST /colibri/v2/conferences/` with `meeting-id` and `create` is
OpenConference, and the same document carries the endpoint creation inside it, so
admission and conference creation are one message rather than two. An adapter
holds the port's separation and issues the messages the bridge expects.

Relays, described in the relay document as pairs of bridges connected with ICE
and DTLS over either SCTP or websockets, are LinkConference. The bridge holds
`relay_id` in its statistics, which is the reference the port passes from one unit
to the other without reading.

ReportCapacity maps to `stress_level` on the `/colibri/stats` endpoint,
documented as the current stress level with zero for no load and one for full
capacity, and values above one permitted. That is the same shape as the load in
[capacity-signal.md](capacity-signal.md), including the decision to let the number
exceed one rather than saturate. The bridge also reports `bit_rate_upload`,
`endpoints`, `largest_conference` and `graceful_shutdown`, which is richer than
the port carries, and the adapter drops the rest.

ReportFaults maps onto `/about/health`, which answers 200 when the bridge deems
itself healthy and a 5xx code when it does not. That is a question the caller
asks rather than a stream the unit pushes, which matches the port's own statement
that silence promises nothing and liveness is decided by asking.

SetReception was not evaluated on this route. The colibri2 REST document read
shows conference and endpoint creation and a dominant speaker query, and no
message that changes what one endpoint receives. Whether the bridge takes such an
instruction over another interface is not answered by the pages read, and this row
is the one most likely to move when the adapter is written.

## What the survey changed in the port

Two things, and both made the port smaller.

Admission returns opaque transport parameters rather than a described transport
description, because one candidate answers with a token and three answer with
transport parameters, and a port that named the second shape would have excluded
the first.

SetReception replaces the whole set rather than adding and removing, because two
of the four candidates express it as add and remove calls and one expresses it as
a per subscriber configuration. A whole set can be turned into either, and a
difference cannot be turned back into a set.

## What no candidate offers

None of the four documents read reports a capacity number derived from what the
unit has committed to send rather than from what it is currently sending. That is
the property [capacity-signal.md](capacity-signal.md) argues is the one that
matters, and it is the property the adapter has to supply for every candidate
here. It is not a reason to reject any of them, and it is a cost that belongs in
the choice made on issue #5.
