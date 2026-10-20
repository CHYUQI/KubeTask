# Liquid glass integration

The UI uses a solid palette, minimal navigation, and translucent surfaces.
No gradient backgrounds or promotional slogans are included.

The navigation material uses the actual `LiquidGlassRenderer`, `buildBottomTabs` and `buildButtons`
from [liquid-glass-webgl](https://github.com/martin65536/liquid-glass-webgl),
the repository behind [the reference demo](https://glass.mt512.qzz.io/).
The Vue adapters are `src/components/LiquidGlassNav.vue` and
`src/components/GlassButton.vue`. They preserve semantic links/buttons,
lazy-load the original WebGL components, respect reduced motion, and dispose
GPU resources on unmount.

Upstream files are pinned to commit
`0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80` in
`src/vendor/liquid-glass-webgl/`; that directory includes provenance and the
upstream AGPL-3.0 license. A copy of the license is also served at
`/third-party/liquid-glass-webgl/LICENSE`.

Existing Task, Workflow, authentication, and SSE API semantics are preserved.
The dashboard links filter the Task list by its existing `phase` parameter.
Trend data displays all history returned by the existing API.

Before editing, the complete `web/` directory (2,761 files, including installed
dependencies and built assets) was archived to:

`D:\VST DEVELOP\KubeTask-backups\frontend-20261020-165055\web-complete.zip`

The adjacent `manifest.json` contains SHA-256 hashes, and the uncommitted patch
and Git status are included in the same backup directory.

## UX adjustments verified with Context7

Official source: Apple Human Interface Guidelines, retrieved via Context7
(/websites/developer_apple_design_human-interface-guidelines), 2026-10-20.

- https://developer.apple.com/design/human-interface-guidelines/layout
- https://developer.apple.com/design/human-interface-guidelines/toolbars
- https://developer.apple.com/design/human-interface-guidelines/typography
- https://developer.apple.com/design/human-interface-guidelines/accessibility
- https://developer.apple.com/design/human-interface-guidelines/materials

Applied: top-down desktop workflow, aligned content and action groups, a
consistent desktop type scale, larger controls for coarse pointers, glass in
navigation/controls only, standard content surfaces, and canvas padding for
native material expansion. Pixel values are web implementation choices, not
a claim that Apple specifies these exact CSS dimensions.

Workflow creation is available at /#/workflows/create. It submits JSON nodes
and optional Task templates through the existing POST /api/v1/workflows API.

## Layout refinement, 2026-10-20

The shared page header now orders navigation, the resource title, and related
actions consistently. Creation pages use a centered 800px maximum column,
aligned labels, grouped fields, and progressive disclosure for Task runtime
options. Workflow metadata uses a responsive six/three/two-column summary so
the dependency graph appears earlier. Compact Task rows link to details;
secondary detail actions are available in a keyboard-accessible More menu.

Typography, content corners, control heights and spacing share one scale.
Canvas overscan is included when sizing narrow navigation to prevent page
overflow. The optical rendering still comes from the pinned upstream source.
These are browser adaptations of HIG principles, not native Apple components.

The previous Context7 lookup remains the source for the initial HIG review.
During this refinement its transport was unavailable, so layout, typography
and toolbar guidance was read directly from Apple's official Markdown pages.

Verification: 18 Node tests and the Vite production build passed; the existing
Dashboard chunk warning (about 537kB) remains. The complete Windows Go
verification pipeline also passed. Browser checks covered 1280px, 390px and
320px widths, form disclosure, the More menu and Escape, lists, details and the
log toolbar. Browser data came from a temporary local fixture, not a Kubernetes
cluster; no real-cluster acceptance is claimed. The fixture is not shipped.
