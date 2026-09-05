# The capacity signal a unit reports

This is the number every part of the scaling loop reads. It answers issue #8.
The port that carries it is in [media-plane-port.md](media-plane-port.md), and it
carries this number and nothing richer.

Nothing in this document is measured. There is no bench yet, issue #2 builds it,
and every figure below is a prediction with the run that will confirm or move it
named beside it. Where a value is a first setting rather than a derivation, it
says so.

## The signal

Its name is load. It is dimensionless, it is one number, and it rises with how
close the unit is to the capacity it was calibrated for.

Zero means the unit is holding nothing. One means the unit is at the capacity
calibration derived for it. Values above one are permitted and are reported
rather than clipped, because a unit that is over its calibration is the situation
the pool most needs to see, and a signal that saturates at one hides exactly the
case where the difference between slightly over and far over decides whether one
new unit is enough.

Being a ratio to the unit's own calibrated capacity is what makes it comparable
between units of different sizes. A load of 0.5 on a small machine and a load of
0.5 on a large one both mean half of what that machine was shown to hold, so the
placer ranks them on one axis without knowing anything about either machine.

## What it is computed from

Three terms, each a ratio with its own denominator, combined by taking the
largest.

The first term is committed egress. The numerator is the sum, over every
reception the unit has accepted, of the target bitrate of the layer it accepted.
The denominator is the egress the unit may use.

The second term is committed packet rate. The numerator is the same sum taken
over packet rates rather than bitrates, because forwarding cost follows packets
and not bytes, and a room of talking heads at a low bitrate can still be a large
number of small packets.

The third term is observed distress: the fraction of the last window in which the
unit could not hand a packet to the operating system when it wanted to. Its
denominator is a fraction that calibration fixes as the point at which the
machine is failing rather than busy.

Committed means accepted, not sent. Both of the first two terms rise at the
moment the unit accepts a reception, which is before the first packet of it
leaves, and that is the whole reason the signal leads rather than trails. A unit
that has just accepted three hundred subscriptions is already at the load they
will cost, whether or not anybody has started talking. A signal computed from the
bitrate actually leaving would still be low at that moment and would only catch up
once the room was already suffering.

The third term cannot lead, and it is not there to lead. It is there so that the
signal cannot report health while the machine is failing for a reason the first
two terms do not model, which is every reason nobody has thought of yet. Because
the terms are combined by taking the largest, this one can only ever raise the
number and never lower it.

## Why the largest and not a weighted sum

A weighted sum is the composite the issue warns about. Its weights are set by
somebody to make a particular case come out right, and when it later misbehaves
nobody can say which input moved, because the answer is arithmetic over all of
them.

Taking the largest has no weights to set. The load is whichever resource is
nearest its limit, which is the constraint that will actually bind first, and
when the number is wrong the question of which term produced it has a single
answer. That answer is not carried over the port, because the port carries one
number, and it belongs in the unit's own diagnostics where somebody debugging
that unit will look.

## Where the denominators come from

Not from an operator. Issue #12 fixes what an operator may set, and the capacity
of a unit is not on that list. The denominators are derived.

The packet rate denominator and the distress denominator come from a calibration
the unit runs against itself, which is the subject of issue #54. Until that issue
lands, only what the calibration has to produce is fixed: the two numbers named
above for the class of machine the unit is running on.

The egress denominator is the exception, and it is the honest one. A machine can
report the speed of its network interface and cannot report what the operator is
paying for beyond it. A unit on a ten gigabit interface behind a one gigabit
uplink would calibrate itself to a capacity that does not exist, and it would find
out during a lecture. So the egress denominator is the smaller of what the machine
reports and the ceiling the operator declared. That declaration is not tuning
under issue #12; it is the operator saying how much they have, which is the
exception that issue already names, and it is the same number the cost work on
issue #14 turns into a bill.

## The decision points

Four, and they are thresholds on the signal rather than decisions in themselves.
Nothing acts on a single sample crossing one of them; the window, the cooldown and
the hysteresis are issue #62 and are deliberately not fixed here.

At 0.60 the unit stops being a preferred home for a new conference. It stays
eligible, because refusing early wastes the pool.

At 0.75 the pool asks for another unit.

At 0.90 the unit takes nothing new, neither a conference nor a participant.

At 1.00 the unit is at its calibrated capacity, and beyond it the quality curve
on issue #4 is expected to start moving.

## Where those numbers come from

Only one of them is chosen. The rest follow from it and from two quantities
nobody has measured yet.

1.00 is not a choice; it is the definition of calibrated capacity, and moving it
would mean recalibrating rather than editing this document.

0.75 is where it is because of the gap between it and 1.00. That gap has to be
large enough that a unit asked for at 0.75 is serving before the load reaches
1.00. Written as an inequality, with R the fastest rate at which load rises on a
unit and T the time from asking for a unit to that unit taking participants:

    1.00 - 0.75  >  R * T

Both R and T are unmeasured. R is what the join storm work on issue #71 produces,
because the top of the hour is when load rises fastest. T is what the
provisioning work on issue #63 produces, and it includes pulling an image onto a
machine that did not exist a moment ago, which issue #110 measures. 0.75 is a
first setting that stands until those two numbers exist, and when they do, this
inequality moves it rather than an argument doing so.

0.90 is where it is because the distance from it to 1.00 has to cover the load
that arrives between the placer reading a stale signal and the unit refusing.
That is one reporting interval of growth at rate R, so the same measurement
settles it.

0.60 is the softest of the four and the one with the weakest derivation. It exists
so that a unit filling up stops attracting whole new conferences before it starts
refusing individual participants, since a conference placed on a unit that is
about to refuse is a conference that will be split across a link for the rest of
its life. Any value between the point where a unit is clearly working and the
point where it starts refusing would do, and this one is a first setting with no
better argument behind it than that it sits in the middle of that range.

## What has to be true, and the run that shows it

The claim this signal makes is that it rises before quality falls. That claim is
false if the quality curve starts moving while the load is still below one, and
the run that would show it is a sweep of participant count on the bench from issue
#2, recording the load and the quality lines from issue #4 in the same record. The
bench run is named `signal-against-curve`, and it is parameterised by the machine
class, so a record from it names both.

What that run has to show, in the form it can fail in:

The load reaches 1.00 at a participant count no higher than the count at which the
first quality line leaves its target. If quality moves first, the signal is wrong
and this document is what changes, not the curve.

The load is monotone in the number of accepted receptions, holding the room shape
fixed. A signal that falls while the room grows cannot be reasoned about by a
placer.

Two units of different sizes running the same room shape report loads whose ratio
matches the ratio of the participant counts they are holding. That is what
comparability means, and it is the property the placer depends on most and the one
least likely to hold by accident.

The calibration run that produces the denominators is named `calibration`, and it
is parameterised the same way. It is the run whose absence makes every figure here
provisional.

## The candidates that were rejected

Participant count. It is the cheapest to produce and it answers the wrong
question. Three hundred people listening to one speaker and thirty people all
publishing video are not the same load, and a signal that gives them the same
number places the second room on a unit that cannot hold it. This is the candidate
most likely to be reached for later under time pressure, which is why it is named
first.

Outbound bitrate as measured. It is closer to the real constraint than a count,
and it trails. A unit at its bandwidth limit is already dropping packets by the
time the measurement says it is at its limit, and everything the pool does in
response then happens after the damage. Committed egress is the same quantity read
before it is spent, which is why the first term is committed rather than measured.

Processor load. It answers for the work of forwarding and says nothing about the
link, so a unit that is bandwidth bound reports a comfortable number while the
room it is holding degrades. It also moves for reasons that have nothing to do
with the conferences, since anything else on that machine raises it. It survives
here only as the distress term, which is a floor and not the signal.

Conference count. It is a count of rooms rather than of work, and the room sizes
this project is built for differ by two orders of magnitude, so it is the
participant count objection with a larger error.

A weighted composite of several of the above. It can be tuned until it is right
for the rooms it was tuned on, and then it is a number nobody can explain when a
different room makes it wrong. The largest of three ratios has no weights to tune
and one answer to the question of why it moved.
