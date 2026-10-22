# Liquid glass integration

Current snapshot: [Shared glass surfaces and performance, 2026-10-21](#shared-glass-surfaces-and-performance-2026-10-21).
Dated sections below retain earlier implementation and verification history;
the current section supersedes native-select and single-atlas descriptions.

The UI uses a solid palette, minimal navigation, and translucent surfaces.
No gradient backgrounds or promotional slogans are included.

The navigation material uses the actual `LiquidGlassRenderer`, `buildBottomTabs` and `buildButtons`
from [liquid-glass-webgl](https://github.com/martin65536/liquid-glass-webgl),
the repository behind [the reference demo](https://glass.mt512.qzz.io/).
The Vue adapters are `src/components/LiquidGlassNav.vue` and
`src/components/GlassButton.vue`. They preserve semantic links/buttons,
lazy-load the original WebGL components, respect reduced motion, and dispose
GPU resources when the page closes. Buttons share one upstream renderer
across route changes; navigation retains its own renderer.

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

## Button rendering correction, 2026-10-21

The upstream React `LiquidGlassCanvas` owns an entire scene; it is not a Vue
DOM component package. KubeTask directly uses its unchanged `buildButtons`,
renderer, shaders, text rasterizer and press/drag springs. The Vue component
provides semantic buttons/links, accessible names, disabled state and native
keyboard/click behavior.

Buttons now share one WebGL renderer rather than compiling a renderer for
every button. Actual source frames, including the animated labels, are copied
to presentation canvases only when upstream requests a render. The adapter
mirrors the source shadow/material/foreground passes to a transparent output
target; it does not copy the opaque scene background over nearby DOM content.
RGB blending remains upstream; alpha accumulation accounts for transparent
composition, and additive highlights preserve existing coverage.

Shadow padding is measured with the upstream transform and shadow geometry,
including press overshoot and drag. CSS brightness filters, button backdrop
pseudo-elements, extra shadows and canvas opacity are removed. Narrow viewports
clip decorative canvas overflow without affecting internal table/graph scroll.

Verification: 25 frontend tests and the production build passed. Regression
tests cover unchanged materials, shadow bounds, atlas separation and transparent
alpha composition. Browser checks include first paint, route changes and the
authentication panel's disabled button. No Kubernetes writes were performed.

## Original tab backing and popup primitives, 2026-10-21

At the user's request, the tab container again uses its original `tabs3`
group, material and press/drag transform. The previous fixed-backing group
override and CSS backing are removed. The 160px transparent render frame
leaves room for expanded native shadows without painting a rectangular
background over page content. The dashboard header now contains Refresh only.

The pinned repository's `src/components/ui/dropdown-menu.tsx`, `popover.tsx`
and `select.tsx` are React/Radix components, separate from its WebGL catalog.
Their Root/Trigger/Portal/Content/Item structure and solid popover styles are
ported to Vue with **Reka UI 2.11.0**, the Vue Radix implementation. These
are Vue ports, not the unmodified React components. Source references:

- https://github.com/martin65536/liquid-glass-webgl/blob/0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80/src/components/ui/dropdown-menu.tsx
- https://github.com/martin65536/liquid-glass-webgl/blob/0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80/src/components/ui/popover.tsx
- https://github.com/martin65536/liquid-glass-webgl/blob/0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80/src/components/ui/select.tsx
- https://reka-ui.com/docs/components/dropdown-menu

The library now manages select/menu positioning, keyboard navigation, focus
return, dismissal and disabled items. Task More uses DropdownMenu; API
authentication uses Popover; all existing BaseSelect consumers use Select.
Application values, including empty-string All filters, retain their API
semantics. No React runtime, gradients or slogans were added.

Verification: 25 frontend tests and the production build passed. The navigation
regression checks now compare the backing with the untouched upstream object
and transform. Browser checks covered tab dragging, the dashboard action,
select keyboard selection and Cron field disclosure, empty filter restoration,
authentication dismissal/focus, and More menu Arrow/End/Escape navigation.
More/filter checks used an isolated read-only fixture on port 18080; it rejects
all non-GET requests, does not proxy to Kubernetes and is not shipped.

## Remove the popup framework, 2026-10-21

The latest implementation removes Reka UI and its 18 added dependency packages,
at the user's request. No replacement UI framework is installed. The previous
section records an earlier snapshot; it no longer describes the current popup
implementation. The WebGL buttons and tab backing remain unchanged.

`BaseSelect` now uses a native HTML select with the existing labels, values,
disabled state and change events. Empty-string All filters need no value mapping;
option descriptions remain available through native title/help text. The browser
and operating system provide the expanded picker and its keyboard behavior.

More and authentication use a shared small Vue `PopupPanel`, with viewport
positioning, outside dismissal and focus return. More keeps Arrow/Home/End
navigation and skips disabled actions. Authentication focuses its input and
retains the existing required-authentication behavior. Popup listeners and resize
observers exist only while open and are removed on close/unmount. No button-to-menu
morph animation is claimed or added.

Verification: all 25 frontend tests, the production build and the complete Windows
Go pipeline passed; generated files have no content changes. No Reka imports or
dependency entries remain. Browser checks covered native keyboard selection and
Cron disclosure, restoring an empty All filter, More keyboard/outside dismissal,
authentication input focus and Escape return, and 320px popup bounds. Menu/filter
checks used the same isolated read-only fixture; no Kubernetes writes were made.
The existing Dashboard chunk warning remains.

## Dashboard glass containers and cache cleanup, 2026-10-21

All dashboard card surfaces now use the pinned repository's unchanged
`buildScrollContainer` card (`sc-card-0`): six linked statistics, both chart
sections, and the disconnected/error notice. The Vue `GlassContainer` keeps
semantic links/sections and accessible DOM text/charts. Only its rectangle,
identifier, clipping bounds and native-page scrolling are adapted; the source
32px corners, lens, saturation, highlight and outer shadow remain intact.
Source: https://github.com/martin65536/liquid-glass-webgl/blob/0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80/src/components/liquid-glass/catalog/build-scroll-container.ts

Cards share one static renderer, separate from animated buttons, and reuse the
existing transparent-output adapter. It draws on initial layout/size changes,
not on a continuous presentation loop or button press. The material DPR is capped
at 1.5; DOM text and charts retain their own resolution. The final card unmount
releases the renderer, framebuffers and WebGL context. Shadow bounds come from
the upstream geometry. No new framework or package was added. The backdrop is
the page's solid color; this adapter does not capture/refraction-render arbitrary
HTML behind the cards.

Verification: 27 frontend tests and the production build passed, including two
new checks for unchanged container material and complete shadow bounds. The full
Windows Go pipeline passed with no generated-file changes. Browser checks covered
all eight ready materials and both charts, refresh, statistics phase links,
route unmount/remount, seven ready surfaces in the disconnected state, and 320px
layout without page overflow. Populated-chart checks used isolated read-only
fixture data; no Kubernetes writes or cluster acceptance are claimed. The
existing Dashboard chunk warning remains (about 541kB).

`npm prune --ignore-scripts --no-audit --no-fund` confirmed no extraneous packages.
`npm cache verify` garbage-collected 946,248,284 bytes and compacted its cache.
Measured npm-cache size fell from 2,487,730,595 to 1,360,495,152 bytes, a net
reduction of **1,127,235,443 bytes (about 1.05GiB)**. Source, current production
output and the original frontend backup are retained.

The three SSR material test servers now disable client dependency pre-bundling
(`optimizeDeps.noDiscovery: true`, empty `include`). After another complete test
run, the existing nine Vite temporary directories were unchanged: no new cache
directories accumulated. Deleting the old Vite cache was rejected by automatic
tool approval with only “blocked by policy” and no detailed reason; its
101,495,508 bytes (96.79MiB) remain. No alternate deletion mechanism was used.

## Shared glass surfaces and performance, 2026-10-21

This closing record describes HEAD `ea815d6` plus the uncommitted working tree.
No backend API, CRD or Controller semantics changed. The upstream source remains
pinned to `0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80`, with its AGPL-3.0 license
and provenance retained. No new dependencies, gradients or slogans were added;
Reka UI and React are absent from the runtime dependencies.

### Surfaces and interaction

`GlassContainer` now covers dashboard cards, creation forms, list/table frames,
detail cards, error/disconnected notices, Workflow graph and node cards, the
log panel, and popup panels. It renders the unchanged upstream
`buildScrollContainer` card material (`sc-card-0`): transparent surface, 32px
corners, refraction height 16, refraction amount -32, blur 0 and saturation 1.5,
with upstream highlight and shadow. The adapter clamps the corner radius to
half the available width/height for compact controls. Ordinary input fields and
layout wrappers remain semantic controls and layout, rather than extra cards.

`BaseSelect` now uses a glass combobox and glass listbox, replacing the native
select from the earlier snapshot. Its small Vue implementation uses the shared
`PopupPanel` and Teleport, with no replacement popup framework. Values/events,
empty-string All filters and disabled options retain their previous semantics.
Focus stays on the combobox with `aria-activedescendant`; Arrow/Home/End move
the active option, typing searches labels/values, Enter/Space selects, Escape
cancels, and Tab commits before normal focus traversal. The popup follows the
trigger width and available viewport height. More and authentication retain
their own focus, dismissal and focus-return behavior. This follows the
[WAI-ARIA combobox interaction pattern](https://www.w3.org/WAI/ARIA/apg/patterns/combobox/);
browser keyboard checks do not constitute a complete assistive-technology audit.

Navigation and button source animations remain. Graph zoom controls and log
search clearing also use `GlassButton`. Popup scrolling is inside the content
area so native material shadows are not clipped into the measured menu bounds.

### Performance and limits

- `container-surfaces.js` bakes static material once per unique layout size and
  DPR, sharing equal-sized surfaces through an LRU cache. Its 4,000,000-pixel
  budget is about 16MB of cached RGBA bitmaps only, excluding DOM canvases,
  renderer buffers and other application memory. Eviction releases bitmap storage.
- An IntersectionObserver with a 200px margin defers offscreen material. Text
  and controls remain available while the material is pending. Rendering is
  scheduled by layout changes, without a continuous static-card animation loop.
- Material DPR is capped at 1.5 for hardware rendering and 1 for software
  rendering, then reduced for large panels to target at most about 2 million
  raster pixels per surface. DOM text and chart resolution are independent.
  DAG zoom uses the original layout dimensions, avoiding material reallocations
  on every graph scale change.
- After 1.5 seconds idle, the static renderer shrinks its drawing buffers to
  1×1 while retaining compiled programs if surfaces remain mounted. If none
  remain, it disposes the renderer/context. Page close clears cached bitmaps
  and GPU resources. Button surfaces copy only changed frames, and duplicate
  releases no longer restart their spring.
- Log auto-scrolling merges work into one animation-frame callback after DOM
  updates and cancels pending work on unmount. It does not discard log records
  or change the SSE protocol.

Transparency still samples the solid page background (`#f5f5f7`); the adapter
does not capture or refract arbitrary HTML behind the surfaces. No measured FPS
or total-memory improvement is claimed. The existing Dashboard bundle warning
remains (537.08kB before compression); no new framework was added to address it.

### Closing verification

- `npm test`: **31/31 passed**, including compact-radius material and LRU reuse,
  eviction/release and large-surface raster-budget tests. Production build passed.
- The full Windows Go pipeline passed: build, vet, controller-gen, build again,
  and all default tests using an absolute envtest asset path. Generated files
  and package manifests/lockfile have no content differences from HEAD.
- Browser checks covered glass selection with Cron/Delay disclosure, All filter
  restoration, Escape cancellation, More keyboard navigation and authentication
  focus return; lists, detail cards, graph/node selection and log surfaces rendered.
  Graph node raster dimensions stayed 405×216 when zoom changed from 100% to 110%.
- A 500-record SSE fixture displayed all records, reached the bottom with
  auto-scroll, and found the final record through search. At 320px viewport
  width the select and popup both measured about 232.67px wide, stayed within
  the viewport and produced no page horizontal overflow. Reopening the menu
  after idle rendered ready material without captured console errors/warnings.

Populated views used an isolated temporary server on port 18080 serving the
production build and read-only fixtures. It rejected non-GET requests, never
proxied Kubernetes, is not part of the repository, and was stopped after QA.
The real development preview remains on port 5173. This is browser UI and local
regression verification, not a new real-cluster acceptance run. The original
frontend backup and prior cache-cleanup record above are retained.
