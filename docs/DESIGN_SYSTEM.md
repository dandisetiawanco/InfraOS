# InfraOS Liquid Glass Design System

## Design Read and Dials

- **Public marketing site**: Architectural Liquid Glass precision style, dial `ENERGY 2 / RHYTHM 3 / MOTION 2`. Varied editorial section compositions without repetitive feature card grids or fake social proof.
- **Signed-in workspace**: Calm native-inspired operational workspace, dial `ENERGY 1 / RHYTHM 2 / MOTION 1`.

## Verified WCAG AA Color Palette

Colors are implemented with CSS `color-scheme: light dark` and `light-dark(...)` driven by `<meta name="color-scheme">` (with a synchronous inline FOUC prevention script and three-state System/Light/Dark controls).

| Token | Light | Dark | Purpose |
|---|---|---|---|
| `--canvas` | `#F3F6F8` | `#10171C` | Level 1 matte backdrop |
| `--surface` | `#FFFFFF` | `#19242C` | Level 2 solid operational surfaces |
| `--text-primary` | `#172A32` | `#EDF4F6` | Primary text (>14:1 contrast) |
| `--text-secondary` | `#586971` | `#A4B5BD` | Secondary text (5.27:1 on light canvas, 5.72:1 on white, 7.46:1 on dark surface) |
| `--accent` | `#237C83` | `#9DE1DD` | Primary deliberate accent (4.90:1 with white, 10.71:1 on dark surface) |

## Four Material Levels

1. **Main canvas (`.infra-canvas`)**: Matte background separating chrome from workspace content.
2. **Solid content surfaces (`.infra-surface`)**: Always used for charts, tables, logs, forms, deployment timelines, and incident editors to preserve readability and contrast.
3. **Translucent Liquid Glass navigation (`.infra-glass-nav`)**: Restrained translucency (`backdrop-filter: blur(18px) saturate(165%)`, subtle specular top inset edge) reserved for the floating collapsible sidebar, workspace toolbar, and public navbar, with solid-surface `@supports` fallback.
4. **Elevated Liquid Glass overlays (`.infra-glass-overlay`)**: Reserved for the Cmd/Ctrl+K command palette, notification center, dialogs, and mobile navigation drawers.

## Typography, Motion, and Accessibility

- Self-hosted **Geist Sans** (`geist/font/sans`) for interface copy and **Geist Mono** (`geist/font/mono`) strictly for logs, IP addresses, commit hashes, metrics, and identifiers.
- Native `<dialog>` top-layer modals with `overscroll-behavior: contain`, native `<button>`/`<a>` semantics, global base-layer `cursor: pointer`, visible `:focus-visible` outlines, 44px touch targets on coarse pointers, container-responsive stacked data tables (`@container`), and full `prefers-reduced-motion` support.
