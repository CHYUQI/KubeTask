# Liquid Glass WebGL

Source: https://github.com/martin65536/liquid-glass-webgl

Reference demo: https://glass.mt512.qzz.io/

Pinned upstream commit: `0b90f9fc1cea8c7e59825f9aff1cbb96674b5b80`.

The renderer, shaders, shape helpers, and catalog sources are copied
unchanged from this revision. The upstream license is included as `LICENSE`.

KubeTask's Vue adapters are src/components/LiquidGlassNav.vue and
src/components/GlassButton.vue. They call the upstream buildBottomTabs,
buildButtons and LiquidGlassRenderer. Labels, bounds, rendering padding and
platform scaling are adapted to the web layout. Optical settings remain
upstream. Semantic DOM links and buttons provide accessibility and fallbacks.
