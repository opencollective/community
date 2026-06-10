# Design system

The visual language takes inspiration from [opencollective.com](https://opencollective.com):
clean white surfaces, generous whitespace, a friendly blue, Inter, soft
radii. Every community can then make it theirs with two colors
([decision 0007](../decisions/0007-community-theming.md)).

## Theming: accent and secondary

A community defines two colors in `/settings/community`:

- **Accent** — primary buttons, links, active states, info surfaces, the
  steward badge. Default: `#1869F5` (Open Collective blue 600).
- **Secondary** — highlights, secondary badges, decorative touches.
  Default: `#6F5AFA` (purple 500).

Implementation: communityd injects CSS custom properties into every page
`<head>`; derived tints are computed with `color-mix()` so one hex value per
color is all a community provides — no build step, no palette editor:

```css
:root {
  --accent: #1869F5;                                        /* from settings */
  --accent-soft: color-mix(in srgb, var(--accent) 8%, white);
  --accent-border: color-mix(in srgb, var(--accent) 25%, white);
  --accent-text: color-mix(in srgb, var(--accent) 70%, black);
  --on-accent: #fff;                                        /* computed server-side */
  --secondary: #6F5AFA;                                     /* same derivation */
}
```

`--on-accent` (text on filled buttons) is computed server-side from relative
luminance: white above the WCAG threshold, ink below. The settings page warns
when a chosen accent fails 4.5:1 contrast either way.

## Fixed tokens (not themeable)

Neutrals and structure stay constant so legibility never depends on taste:

| token | value | use |
|---|---|---|
| `--ink` | `#141415` | headings, primary text |
| `--text-2` | `#4D4F51` | body text |
| `--text-3` | `#75777A` | secondary text |
| `--text-4` | `#9EA0A3` | hints, timestamps |
| `--border` | `#EAEAEC` | card borders, dividers |
| `--border-2` | `#DCDDE0` | input borders |
| `--bg-page` | `#F9FAFB` | page background |
| `--bg-card` | `#FFFFFF` | cards, nav |
| `--success / --danger` | `#0EA755` / `#CC2955` | semantic states |

(Values are the Open Collective neutral/green/red ramps.)

## Typography

- **Inter**, self-hosted (no Google Fonts request — privacy and offline
  installs). Weights 400 and 500 only.
- Body 14px/1.6. Headings: 24px hero, 18px page title, 15–16px card title,
  all weight 500 with `-0.2px` letter-spacing.
- Sentence case everywhere. No all-caps labels.

## Components

- **Buttons** — 8px radius. Primary: filled `--accent`. Secondary: white,
  1px `--border-2`. Destructive: outline, `--danger` on hover/confirm.
- **Cards** — white, 1px `--border`, 10–12px radius, 16–18px padding.
- **Inputs** — 8px radius, 1px `--border-2`, focus ring in accent.
- **Role badges** — 99px pills, 11px/500. Colors come from the role's own
  configurable color (defaults: admin purple, steward accent, moderator
  green, member neutral).
- **Info/success panels** — accent-soft or green-tinted backgrounds with
  matching darker text, used in the wizard and confirmations.

## CSS architecture

One hand-written stylesheet (`assets/community.css`, target < 300 lines),
embedded in the binary, served with a content-hash query for cache busting.
No Tailwind, no preprocessing. htmx provides interactivity; JS is reserved
for the chat websocket, the code-input boxes and clipboard copy.

## Voice

Short sentences, plain words, no jargon on public pages. Nostr concepts are
explained in passing ("your key never leaves this server") rather than
assumed. Error states always say what to do next.
