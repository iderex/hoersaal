# What this project is for, and what it is not for

The notice in the tree covers lawful use in general terms. This is the specific
version, and it exists because software that carries hundreds of people's audio
and video gets used for things its authors did not intend. It answers issue #109.

It is written plainly and without threat. Most of the people who read it are
trying to do the right thing and want to know where the edges are.

Nothing here is legal advice. It says what the project is built to do and what it
will not be built to do, which is a different question from what any particular
deployment is allowed to do.

## What it is for

An operator running meetings and lectures for their own people, on hardware they
control. A university department, a hospital, a school, a public body, a company.
The people in the room are people the operator already has a relationship with,
and the operator is the one answering for the service.

The two properties everything else is arranged around are that a room of a few
hundred works without anybody tuning a bridge, and that the conversation stays on
the operator's own machines. The second is why there is no federation between
installations: two installations of this software do not hold a room between
them, do not exchange participants and do not assert anybody's identity to each
other, which is decided in
[decisions/federation.md](decisions/federation.md) rather than left as a
disposition.

## What it is not built for, and will not be

None of these is a missing feature. Each is a thing the project will decline, and
saying so now is cheaper for everybody than saying so later to somebody who has
already built on the assumption.

Watching people. There is no facility for an operator, or anybody else, to listen
to or view a room without being in it and visible in it as a participant. A
moderation action is recorded because a person needs to answer for it later, and
that record is of the action and not of the conversation, which is what issue #88
fixes.

Analysing what is said. No transcription, no keyword detection, no sentiment or
attention scoring, no voice or face identification. The logs are built to carry no
conversation content at all, which is issue #85, and the metrics are built to
carry no personal data in a label, which is issue #83. A feature that needed
either of those to change is a feature this project does not want.

Giving a third party a copy of a room. There is no interception interface, no
duplicate stream for an observer outside the room, and no plan to add one. Where
an operator is legally compelled to do something of this kind, that is between
them, their lawyers and their regulator, and the project will not build the
mechanism that makes it convenient.

Running a room for people the operator has no relationship with. Admission is
built around an operator's own accounts and room credentials. Using it as a public
broadcast platform for an audience nobody is accountable for is outside what the
capacity work, the moderation work and the abuse surface have been designed
against, and the project will not take on that shape.

Reaching participants who are not in the room. There is no dial-out to somebody
who has not joined, no automated invitation sending to lists, and nothing that
initiates contact with a person on the operator's behalf.

## Where the operator's obligations begin

They begin the moment the service is run for anybody other than the operator
themselves.

At that point the operator is the one holding other people's conversations, and
the questions that follow are theirs to answer. What they tell participants
before they join. What they keep, for how long, and what removes it. Who among
their own staff can moderate a room and what record that leaves. What happens
when a participant asks what is held about them. Whether their own rules and the
rules of their sector permit the meeting they are about to hold.

The project's part is to make those questions answerable rather than to answer
them. What data exists, where it is held, for how long and what removes it is the
data protection statement on issue #103, and it is written for an operator who has
to answer to their own users and their own regulator.

This paragraph is not legal advice and is not a substitute for any. It is a
statement of where the line between the project and the deployment falls.

## What the project will help with

Running it, sizing it, and understanding why it did what it did. Reports of it
failing, reports of it being slower than the published figures, reports of it
holding data it should not, and reports of a security defect through the route
issue #106 sets up.

It will not help with adding any of the things in the section above, whether the
ask arrives as a feature request, a patch, a fork it is asked to bless, or a
consulting arrangement. The answer is the same in each case and it is not a
negotiation.

The licence permits a fork that does any of it. Nothing here changes that, and
nothing here is a claim to control what somebody else does with their own copy.
It says what this project is, and a fork that removes these boundaries is a
different project with a different name.
