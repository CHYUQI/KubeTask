# Liquid Glass WebGL

Source: https://github.com/martin65536/liquid-glass-webgl

Reference demo: https://glass.mt512.qzz.io/

Pinned upstream commit: `0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80`.

The retained renderer, shaders, helpers, and catalog sources are copied
unchanged from this revision. The upstream license is included as `LICENSE`.

This is a dependency subset, not the complete demo repository. On 2026-10-22,
25 modules unreachable from KubeTask's source and tests were removed: unused
catalog demo builders and their helpers/entry point, continuous-sdf.ts,
stroke-mask.ts, shapes/continuous-corners.ts and text-sdf.ts. The used
buildBottomTabs, buildButtons and buildScrollContainer builders and their
transitive dependencies remain. Recover omitted examples from the pinned
upstream revision above if needed; do not restore the full demo for one widget.

KubeTask's Vue adapters are src/components/LiquidGlassNav.vue,
src/components/GlassButton.vue and src/components/GlassContainer.vue. They
call upstream buildBottomTabs, buildButtons, buildScrollContainer and
LiquidGlassRenderer. Labels, bounds, rendering padding and
platform scaling are adapted to the web layout. Optical settings remain
upstream. Semantic DOM links and buttons provide accessibility and fallbacks.
