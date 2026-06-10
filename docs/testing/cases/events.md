# Test cases — Events channel and calendar feed

Flow reference: [nostr/channels.md](../../nostr/channels.md) § events.
The Events channel inherits all CHAN-* behavior; these cases cover the
template and the calendar.

### EVT-01 — a member creates an event
Given the Events channel is enabled and member @dan submits the event form
(title, description, start, end, location, repeats monthly)
Then a kind 31923 event signed by dan's key exists with the channel `h` tag,
NIP-52 start/end/location tags and an `rrule` tag (FREQ=MONTHLY)
And it appears in the channel list as pending

### EVT-02 — template validation
Given an event whose end precedes its start, or whose `rrule` is malformed
Then submission is rejected with the specific violation, nothing signed
And an all-day event (date, no time) produces kind 31922 instead of 31923

### EVT-03 — approval follows the channel policy
Given dan's pending event
Then dan's own approval does not count
When steward @alice approves (kind 4550)
Then the event is approved — and visible to logged-out visitors

### EVT-04 — the ICS feed serves approved events
Given approved, pending and declined events, one approved event recurring
When `/channels/events.ics` is fetched (no authentication)
Then the response is `text/calendar` with one VEVENT per approved event
(DTSTART, DTEND, SUMMARY, LOCATION, and RRULE for the recurring one)
And pending and declined events are absent
And the feed updates when a new event is approved

### EVT-05 — homepage upcoming events section is conditional
Given the Events channel disabled, or no approved upcoming events
Then the homepage shows no events section
Given it is enabled with one approved future event and one approved past event
Then the section appears with only the future event, plus the ICS subscribe link
And a recurring event with past start shows its next occurrence

### EVT-06 — cancelling an approved event
Given an approved upcoming event
When the author or an approver cancels it (kind 1985 `cancelled` label)
Then it leaves the homepage and is marked CANCELLED in the ICS feed
And the thread remains readable with a cancelled badge

### EVT-07 — event threads discuss like any thread
Given an approved event
Then members reply (kind 1111) and react (kind 7) as in CHAN-05/06
And the reply count shows in the channel list
