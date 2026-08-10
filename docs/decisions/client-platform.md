# The client platform

This answers issue #74. It fixes what the client is written for, what a browser
has to be able to do before this service works in it, what the choice costs this
repository, and how a client is tested on a machine with no display.

It writes no client. That work is issues #75 to #81 and this is the document they
answer to. It chooses no codec and no layer mechanism, which is issue #48, and
the one number below that depends on that choice says so rather than guessing it.

## The platform

The browser, reached from a link, with nothing installed.

The room this service exists for is three hundred people arriving minutes before
a lecture, on devices they did not prepare and frequently may not install
software on. A managed laptop is the ordinary case in a university or a hospital
rather than the awkward one, and a lecture that starts with a hundred downloads
starts late. Every other candidate puts an install between the link and the room:
a native application per operating system, a desktop shell around web code, a
mobile application through two stores. None of them removes the browser client
either, because the person on a borrowed machine still has to get in, so each is
the browser plus a second client rather than instead of one.

## What the choice gives up

The media stack belongs to somebody else. Which codecs are offered, how
congestion control behaves, when a decoder gives up and whether a hardware
encoder is used are the browser's decisions, and a defect in one of them is a
defect this project can report, measure and route around rather than fix. That is
the largest single thing the choice costs, and it is why issue #48 is a policy
over what browsers offer rather than a choice of encoder.

Three engines rather than one, and they differ where it is most expensive to find
out: device permission dialogs, what a hidden tab is allowed to do, and what a
connection does when a laptop suspends. The floor below is stated per engine for
that reason rather than per branded browser.

Anything touching real media needs a real browser with a real device to be
certain of, and no check in this repository may require one. That is the third
test layer below and it is what issue #51 exists for.

## What a browser has to be able to do

Four capabilities, each required by a decision that is already landed rather than
by a preference recorded here.

A WebSocket to the origin the page came from, because that is the transport in
[signalling-transport.md](signalling-transport.md), which is one HTTPS listener
and no second port.

One connection carrying many remote tracks, because a subscriber in
[media-plane-port.md](media-plane-port.md) receives sources from many publishers
through one unit. In a browser that is `RTCPeerConnection.addTransceiver` and the
transceiver interface around it.

A camera and a microphone for the participants who publish, which is
`getUserMedia`. It is a requirement on the few and not on the three hundred, and
issue #76 already asks that a participant who is not publishing is never asked
for either.

More than one encoding of one video source from a publisher, because a layer in
[media-plane-port.md](media-plane-port.md) is one of the encodings a publisher
sends and a subscriber receives at most one of them per source. How a browser
produces those layers is issue #48's to choose, and the two answers do not carry
the same floor.

### The floor

Read from the browser compatibility data the Mozilla project publishes, at the
state that repository was in when this was written. These numbers are read from
that source rather than measured here, because there is no client in this tree to
run against a browser:

    curl -sS https://raw.githubusercontent.com/mdn/browser-compat-data/main/api/RTCPeerConnection.json | python3 -c "import json,sys
    d = json.load(sys.stdin)['api']['RTCPeerConnection']['addTransceiver']
    for name, node in (('addTransceiver', d), ('init.sendEncodings', d['init_sendEncodings_parameter'])):
        s = node['__compat']['support']
        print(name, {b: (s[b]['version_added'] if isinstance(s[b], dict) else s[b]) for b in ('chrome', 'edge', 'firefox', 'safari')})"
    addTransceiver {'chrome': '69', 'edge': 'mirror', 'firefox': '59', 'safari': '11'}
    init.sendEncodings {'chrome': '69', 'edge': 'mirror', 'firefox': '110', 'safari': '14.1'}

    curl -sS https://raw.githubusercontent.com/mdn/browser-compat-data/main/api/MediaDevices.json | python3 -c "import json,sys
    s = json.load(sys.stdin)['api']['MediaDevices']['getUserMedia']['__compat']['support']
    print({b: (s[b]['version_added'] if isinstance(s[b], dict) else s[b]) for b in ('chrome', 'edge', 'firefox', 'safari')})"
    {'chrome': '53', 'edge': '12', 'firefox': '36', 'safari': '11'}

So a participant who only listens and watches needs Chrome 69, Firefox 59 or
Safari 11. Edge is recorded there as mirroring Chrome rather than carrying a
number of its own, so its floor is its first Chromium release.

A participant who publishes needs more, and this is where issue #48 arrives. If
the layers are produced by simulcast, the floor is the second row above, Chrome
69, Firefox 110 and Safari 14.1, and Firefox is the most recent of the three. If
they are produced by scalable coding inside a single encoding instead, the floor
is a different set of numbers and this document does not state them, because they
were not read on this route. Issue #48 picks the mechanism and brings its floor
with it.

A floor is not a support promise. What this project supports is what its own runs
cover, and today they cover nothing, because there is no client. The promise
arrives with the headless job on issue #75 and is the set of versions that job
pins.

## What it costs this repository

A second toolchain and a second dependency graph, and the cost is paid rather
than absorbed.

Today this is one language, one suite and a graph with nothing in it. A browser
client adds a second language, a second package manager with its own lock file
and its own supply chain, and a browser binary something has to fetch before it
can check anything. That widens what a reviewer and an operator are trusting, and
it lands on three issues that each read one graph today: issue #20 locks the
graph, issue #21 produces the bill of materials, and issue #102 produces the
third-party notices. Each becomes two graphs on the day the first client file
lands.

It also reaches the top level. `docs/repository-layout.md` names four directories
and says a fifth is a change to that document before it is a change to the tree,
and `internal/arch` refuses one the document does not name. So the first client
file cannot land quietly: it arrives with an edit to that document and the check
goes red until it does. The arrangement that keeps the licence question cheap,
which is that the client libraries and the protocol definitions sit in their own
top-level directories from the commit that first creates them, is on issue #1 and
is not restated here.

Two things hold the cost down and both follow from decisions already taken.

The wire needs no build step. The protocol is JSON text over a WebSocket, so a
client speaks it with what the language already has, and no schema compiler, code
generation or generated stub is between a client and the server. The second
toolchain above is therefore a choice about how the interface is built and not a
requirement of the protocol.

The part of the client that decides things needs no browser to test, which is the
next section, so the expensive layer is the smaller one.

## How a client is tested without a display

Three layers, and this record says which of them a check here may require.

Everything that is a decision rather than an interface is tested as ordinary code
with no browser. What the client decides has the same shape as what the server
decides: which state a session is in, what to show for each refusal, how the
question queue is ordered, when to try to reconnect and what to do when the
window has passed. Those are functions of their inputs and they are tested the
way `internal/placement` is tested. Most of the client's defects will be there.

The interface is tested in a headless browser, which is a real engine with no
display, driven by a program, with capture faked by the engine rather than
attached. That layer is where the join path on issue #76 and the accessibility
floor on issue #80 are asserted, and a check here may require it, because it
needs no display, no device and no elevation. Which engines, which driver and
which versions satisfy that is issue #75, and is pinned there rather than
described here.

Real media on real hardware is the third layer and no check here may require it.
It goes to the media integration harness on issue #51, which is a separate runner
with the devices attached, and until that exists the honest move is to say in a
pull request that the case is not covered.

## The protocol, checked against this choice

The protocol is four landed pieces and one that is not written. The transport and
the envelope are issue #31, in [signalling-transport.md](signalling-transport.md)
and `internal/wire`. How somebody proves who they are is issue #33, in
[accounts-and-room-credentials.md](accounts-and-room-credentials.md) and
`internal/roomcred`. What a client is told about the room is issue #37, in
[presence.md](presence.md) and `internal/presence`. The admission handshake that
carries a credential into a room is issue #35, and it and the message types the
exchanges own are the part that does not exist. So this is a check of what is
there and a rule for what comes.

Five assumptions about a particular client are already in it, and every one of
them is the browser's. None is a defect and none was made here. Two of the five
name a browser in the sentence that makes the choice, which is what turns this
condition into a check with an answer rather than a formality:

    git grep -n 'fragment of a link\|cookie marked HttpOnly' origin/main -- docs/decisions/
    origin/main:docs/decisions/accounts-and-room-credentials.md:150:also why the credential belongs in the fragment of a link rather than in its
    origin/main:docs/decisions/accounts-and-room-credentials.md:209:cookie marked HttpOnly, Secure and SameSite=Lax.

The transport argument is a browser argument. A WebSocket upgraded from an
ordinary HTTPS request was chosen for what a reverse proxy, a TLS terminator and
an inspecting gateway will carry, and that is the path a page takes. A native
client could have opened its own port and would have wanted a different answer,
so the transport was already chosen for this platform before this document chose
it.

The liveness design assumes an endpoint that answers a ping without page code
doing it. The server pings every twenty seconds and expects the answer within
twenty. The interface a browser gives page script has no ping and no pong in it:

    curl -sS https://raw.githubusercontent.com/mdn/browser-compat-data/main/api/WebSocket.json | python3 -c "import json,sys
    print(sorted(k for k in json.load(sys.stdin)['api']['WebSocket'] if k != '__compat'))"
    ['WebSocket', 'binaryType', 'bufferedAmount', 'close', 'close_event', 'error_event', 'extensions', 'local_network_access', 'message_event', 'open_event', 'protocol', 'protocol_rfc_6455', 'readyState', 'send', 'url', 'worker_support']

and the answer is required of the endpoint rather than of the page:

    curl -sS https://www.rfc-editor.org/rfc/rfc6455.txt | sed -n '2036,2043p'
    5.5.2.  Ping

       The Ping frame contains an opcode of 0x9.

       A Ping frame MAY include "Application data".

       Upon receipt of a Ping frame, an endpoint MUST send a Pong frame in
       response, unless it already received a Close frame.  It SHOULD

So in a browser this is not the client author's to get wrong, which is why the
design works there. In a client written against a socket library it is theirs, and
a library that does not answer leaves a client the server stops hearing from and
closes on the read deadline, with nothing on the client side reporting why. That
sentence is written nowhere else on this board and a third-party client author
needs it.

The credential travels in the fragment of a link and the reason given is a
browser's behaviour: a fragment is not sent to the server, so it stays out of an
access log. A client that is not a page has no address bar, no history and no
referrer, so the argument does not reach it and it holds the credential
somewhere else. The place that is built is the join route on issues #35 and #76,
and the document already says so.

The session is a cookie, and the three attributes it is marked with are a
browser's mechanism rather than the protocol's. `HttpOnly` means page script
cannot read it, `Secure` means the transport, and `SameSite=Lax` bounds what
another site can make a browser do with it. None of the three exists for a client
holding a string in a variable, which is the operator route rather than the
route the three hundred take, and it is the sharpest of the five.

Text framing suits this platform in a way the transport document did not claim.
It argues text for an operator who wants to read what is happening; in a browser
it also means the client needs nothing generated to speak the protocol, which is
the second half of the cost paragraph above.

Presence asks one thing of a client and it is not a browser thing:

    git grep -n 'knows it missed one' origin/main -- docs/decisions/presence.md
    origin/main:docs/decisions/presence.md:28:revision `n-2` knows it missed one and asks for a page, which is the only place

A client that sees a revision jump asks for a page, and any client can do that.
What presence does assume is that its reader is an interface a person looks at:
the attending set is capped at eight because eight is a handful somebody can
attend to, and a client that is not a person's window has a different reason to
want the room and asks for a page like anybody else. That is an assumption about
a client and not about a browser, so it is named here rather than corrected.

The rule the exchanges are held to, so that the protocol work does not assume one
client by accident: an exchange may assume the transport above, because every
client reaches this service through it. It may not require something only a
browser has, and where one genuinely does, the exchange says so where the message
is defined rather than leaving a third-party client author to find it by being
disconnected.

## If a native client is wanted later

The wire does not change. JSON over a WebSocket on the HTTPS port is available to
any language, and the envelope refuses an unknown member rather than an unknown
client.

Four things do change, and they are the assumptions above read backwards. The
liveness answer becomes the client author's. The media stack becomes theirs too,
which is this document's first cost in reverse: a native client may fix a codec
defect and has to carry a codec. Holding a credential and holding a session
become theirs, since a fragment and a cookie are the browser's answers to
questions every client still has to answer somehow, and the reasoning behind both
is written against a browser rather than against a client. And the install
returns, which is why a native client is an addition for people who will pay that
price and never the only way in. The three hundred arriving from a link are the
requirement everything here is derived from.

## What is decided here and what is not

Decided: the browser; the four capabilities; the floor, conditional on issue #48
for a publisher; the three test layers and which of them a check may require; and
that the second toolchain is paid for knowingly.

Not decided here: which framework, which package manager and which headless
driver, each of which is the means check of the change that introduces it; the
codec and layer policy, which is issue #48 and is what turns the publisher floor
into one number; the licence of the client libraries and the protocol
definitions, which is issue #1's; and the versions the checks pin, which arrive
with issue #75.
