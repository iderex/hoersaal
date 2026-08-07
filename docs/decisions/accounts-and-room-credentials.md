# Accounts, sessions and the room credential

This says how somebody proves who they are to this service. It answers issue
#33. The credential half is built in `internal/roomcred/roomcred.go` and every
number that package refuses against is fixed here rather than in a comment
beside the code, so the two cannot end up holding different values.

Nothing here is measured, and nothing here needs to be. These are choices about
what is refused, and the evidence for them is the argument rather than a run.

## Two populations, two routes

A lecture holds two kinds of people and they cannot be authenticated the same
way.

The people who run the room are known to the installation. They are few, they
come back, and what they can do is dangerous, so they hold an account with a
password and a session.

Most of the people in the room were sent a link. They are many, they arrive once,
and requiring three hundred of them to register is how a lecture tool stops being
used. They hold a room credential, which admits the holder to one conference in
one role for a bounded time and to nothing else.

Both routes exist in every installation. Which of them a given room accepts is a
property of the room and is decided when the room is made, and this document does
not fix that policy because room admission is issue #35.

## The room credential

What it is, in one sentence. A short string of base64url, safe to put in a link,
carrying the conference, an optional subject, a role and a window, with a
signature over all of it.

It is a bearer credential. Whoever holds the bytes may use them. That is a
property of sending a link to three hundred people rather than a defect, and what
bounds the damage is what is written inside the bytes.

### What is inside, and what is not

Inside: the layout version, the conference, a subject, a role name, and the two
instants that bound the window. The signature covers all of it, including the
version, so no field can be edited by the holder without the credential ceasing
to verify.

Not inside: anything that identifies a person. The subject is an identifier this
installation made up for this occasion, never a name, an address or a directory
identifier. The reason is where the bytes travel. A credential in a link is in a
browser history, in a referrer header if the page ever links outward, in whatever
proxy log sits between the reader and the service, and in the message the link
was sent in. Every one of those is a place personal data would be held by
somebody who never assessed it, and the cheapest way not to hold it there is not
to put it there.

The subject may also be empty, which is the case of one link sent to a group.
That is supported deliberately, because refusing it would push an installation
into putting something identifying in the field instead. What an empty subject
costs is that the credential says only that its holder was sent the link, so
anything that needs to tell two attendees apart takes it from the session
established at admission rather than from the credential.

### Signed rather than encrypted, and with a shared key

The credential is signed with HMAC-SHA256 and is not encrypted. Its contents are
a conference identifier, a role and a window, none of which is a secret from its
holder, and encrypting them would only hide from the holder what the holder is
about to use.

A shared key rather than a public key signature, because the only party that has
to verify a credential today is the same installation that minted it. A public
key signature buys verification by somebody who may not mint, which nothing here
needs. If a later deployment shape needs it, for instance a unit verifying a
credential without holding the minting key, that is a second layout under a new
version byte rather than a change to this one, and the version is the first byte
so the two are told apart before anything else is read.

### The numbers, and why each one

A key of at least 32 bytes. HMAC-SHA256 has a 256 bit output, and a key shorter
than the output is the part an attacker would go at first. The package refuses a
shorter key on both sides rather than accepting one and being weaker than it
looks.

A token of at most 1024 bytes, refused before any of it is decoded. The largest
credential this layout can hold is well under that, so the limit is not a
constraint on legitimate use; it is there so that a stranger cannot make this
process do work proportional to what they sent.

A text field of at most 128 bytes. Conference identifiers, subjects and role
names are made by this installation and are short. The limit is checked when a
credential is minted and again when one is read, so a credential that was
somehow minted elsewhere still cannot carry a field this build did not expect.

A lifetime of at most 12 hours. A credential is for one occasion. Twelve hours
covers a day of sessions in one room, which is the longest single occasion a
lecture hall actually has, and it bounds what a link forwarded to somebody else
is worth to them. Where a room runs longer than that, the answer is a new
credential rather than a longer one.

### Revocation, which this does not have

A signed bearer credential cannot be withdrawn before its window closes without
somewhere to write down that it was withdrawn, and nothing here writes anything
down. This is stated rather than solved.

What bounds it is that a credential admits and does not keep anybody in the room.
Removing somebody who is already in a conference is the server-enforced
moderation on issue #38, and it acts on the session rather than on the
credential. Closing a room to everybody is the same mechanism. So the case a
missing revocation actually leaves open is a person who has not joined yet and
whose credential has not expired, and the operator's answer to that today is to
end the room.

### Key rotation, which this does not have either

The package holds one key. An installation that needs to change its key today
invalidates every credential minted under the old one at the moment it does so.

What would remove that is a verifier holding a previous key and accepting a
signature from either while minting only under the current one, for a period no
shorter than the longest lifetime above. That is a small change to this package
and a larger question about where an operator's keys are held and how they are
handed over, which is issue #86. Nothing here depends on it, and this paragraph
is the whole of what is owed.

## What each route is exposed to

### The room credential route

Forgery. Somebody writes their own credential naming themselves a presenter.
Refused by the signature, which covers every byte including the role.

Replay into another room. A credential for one lecture presented to another.
Refused by the conference binding, and refused at the place that reads the
credential rather than by a caller remembering to compare afterwards: verifying
takes the conference being entered as an argument, so a caller that forgot would
not compile.

Replay after the occasion. Refused by the window, judged against a clock the
verifier is handed rather than one it reads for itself, which is what makes the
refusal testable.

Forwarding. A person sends their link to somebody else. Not refused, and it
cannot be by any property of the bytes. What bounds it is the window and the fact
that the credential names one conference and one role. An installation that needs
more than that needs an account, which is the other route.

The link leaking into a log or a history. Bounded by what is inside the
credential, which is why the subject is an identifier rather than a person. It is
also why the credential belongs in the fragment of a link rather than in its
query, since a fragment is not sent to the server by a browser and does not reach
an access log. That is a property of the client and of the join route rather than
of this package, and issues #35 and #76 are where it is built.

Guessing. There is nothing to guess. A credential is not a short code and the
signature is not truncated.

### The account route

Password guessing, one account at a time. Bounded by the delay below rather than
by a lock, for the reason given there.

Credential stuffing, which is the same passwords from somewhere else tried
against many accounts. The per-account delay does little against it, since each
account sees one attempt. What does help is refusing passwords that are known to
have been breached, which is the blocklist below, and watching the rate of
failures across the installation rather than per account.

Session theft. Bounded by the session lifetimes below, by the cookie flags, and
by the session identifier being long enough not to be guessed. A stolen session
is not detectable here and this document does not claim it is.

Phishing. Not addressed by anything in this document. An installation whose
operators are worth phishing wants a second factor, and this document does not
design one; it is named as absent rather than left to be discovered.

The operator themselves. A self-hosted service places everything in the hands of
whoever runs it, and that is a property of self-hosting rather than a threat to
be refused. What is owed is that the documentation says so, which is issue #105.

## Password storage

Argon2id, with a memory cost of 64 MiB, three passes and four lanes, and a
16 byte salt per password from the operating system's source. The reason for a
memory-hard function rather than a fast hash is the economics of the attack: an
attacker with the stored values runs them on hardware built for the purpose, and
memory is the part of that hardware that does not get cheap quickly. The reason
for those figures rather than larger ones is that they are paid on the server on
every sign-in, and a lecture hall installation is not a machine with cores to
spare at the top of the hour.

The library that provides it is not in this tree, and adding it is part of the
account work rather than of this issue. That is the point at which the means
check on it gets made.

A stored value is compared in constant time, never with an ordinary string
comparison, and never logged at any level.

A password of at least 12 characters, with no maximum below 128 and no
composition rules. Composition rules produce predictable passwords, because
people satisfy them the same way. Length and a refusal of passwords known to have
been breached do more, and the second of those needs a list the installation
holds rather than a service it calls, since asking a third party about a password
is the opposite of what this project is for.

## Sessions

A session identifier of 256 bits from the operating system's source, held in a
cookie marked HttpOnly, Secure and SameSite=Lax.

Idle lifetime of 30 minutes. An operator account is a browser tab in a room other
people walk through, and thirty minutes is short enough to matter and long enough
not to end a session in the middle of running a lecture.

Absolute lifetime of 12 hours, after which signing in again is required whatever
the tab has been doing. This is what bounds a stolen session that keeps itself
alive.

The identifier is replaced when what the session may do changes, so a session
identifier captured before a privilege change is not one afterwards.

## Failed attempts

A delay per account rather than a lock. After the first failure the next attempt
on that account waits one second, then two, four, eight, sixteen, and thereafter
thirty seconds, and the counter returns to zero after fifteen minutes with no
failure or after one success.

A lock is refused on purpose. An account that can be locked by somebody who knows
its name is an account that can be kept out of its own room five minutes before a
lecture, which turns a guessing defence into a way of stopping the lecture. A
delay costs an attacker the same time and costs the person who mistyped their
password almost nothing.

Beside it, a limit of 20 attempts a minute from one source address across all
accounts, which is what the per-account delay does not cover. The counter is held
in memory and is lost when the process restarts, which is a real bound and is
stated rather than glossed: an attacker who can restart the service has already
won something larger.

A failed attempt is recorded as a count and not as a line naming the person who
made it. What may appear in a log at all is issue #85.

## The seam for an external identity provider

Most installations of this software sit inside an organisation that already runs
something that knows who its people are, and being able to use it is the
difference between a service the organisation adopts and one it tolerates.

The seam is the account route and nothing else. One question is asked, which is
whether this person is who they say, and everything downstream takes the answer
and never the method. The room credential route does not touch it, so a
deployment with no provider is complete rather than degraded, and that is what
"nothing depends on it existing" means here.

What connecting one would take, so the size of it is known rather than assumed.
Discovery of the provider's configuration, the authorisation code flow with PKCE,
fetching and refreshing the provider's signing keys, mapping what the provider
says about a person onto the roles this service has, deciding what happens to a
local session when the provider ends its own, and deciding what the service does
when the provider cannot be reached at the moment a lecture starts. The last of
those is the one that decides whether an organisation trusts this arrangement,
and it is a design question rather than a configuration one.

None of that is built, no issue on the board holds it today, and this paragraph
is the whole of the record.

## What this does not decide

What powers a role name grants is issue #34. A role here is a name carried
faithfully and given no meaning.

The exchange in which a credential is presented, what the client is told when it
is refused, and how the media plane credential that follows carries no power the
room credential did not, are issue #35.

Where an installation's key is held and how it reaches the process is issue #86.

Whether an installation lets anybody create an account is a policy this document
leaves to whoever runs it.
