# 0007 — per-community theming via two colors

Status: accepted (2026-06-10)

## Context

Every community should feel like *its own place*, not an instance of
someone else's product. Full theme systems (custom CSS, palette editors,
template overrides) are powerful but create an unmaintainable support
surface and break with every release.

## Decision

A community defines exactly two colors in `/settings/community` — **accent**
and **secondary** — defaulting to the Open Collective-inspired palette.
communityd injects them as CSS custom properties; all tints and borders are
derived in CSS with `color-mix()`; text-on-accent is computed server-side
from relative luminance with a contrast warning in settings. Neutrals,
typography, spacing and components are fixed
([design/design-system.md](../design/design-system.md)).

## Consequences

- Real identity with two decisions and zero build tooling; themes can never
  break layout or accessibility floors.
- Role badge colors follow the same one-hex-plus-derivation rule, so the
  whole UI stays coherent automatically.
- Deliberate ceiling: no custom fonts, no custom CSS, no logos beyond the
  community icon in v1. Communities wanting more are a signal for a future
  ADR, not a settings flag today.
