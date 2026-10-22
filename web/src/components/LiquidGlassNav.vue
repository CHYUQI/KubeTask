<template>
  <nav
    ref="host"
    class="liquid-nav"
    :class="{ ready, 'is-dragging': dragging }"
    aria-label="主导航"
    @pointerdown="startPointer"
    @pointermove="movePointer"
    @pointerup="endPointer"
    @pointercancel="cancelPointer"
    @lostpointercapture="cancelPointer"
    @click.capture="guardClick"
    @dragstart.prevent
  >
    <canvas ref="canvas" aria-hidden="true" @webglcontextlost="loseContext" />
    <router-link
      v-for="(item, index) in items"
      :key="item.to"
      :to="item.to"
      :class="{ selected: selected === index }"
      >{{ item.label }}</router-link
    >
  </nav>
</template>
<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { createGlassOutput } from "./glass-output.js";
import {
  prepareNavigationElements,
  NAVIGATION_CATALOG_HEIGHT,
  NAVIGATION_FRAME_HEIGHT,
} from "./navigation-material.js";
const route = useRoute(),
  router = useRouter(),
  host = ref(null),
  canvas = ref(null),
  ready = ref(false),
  dragging = ref(false);
const items = [
  { label: "总览", to: "/" },
  { label: "任务", to: "/tasks" },
  { label: "工作流", to: "/workflows" },
];
const selected = computed(() =>
  route.path.startsWith("/workflows")
    ? 2
    : route.path.startsWith("/tasks")
      ? 1
      : 0,
);
const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
let renderer,
  output,
  buildBottomTabs,
  defaults,
  palette,
  observer,
  disposed = false;
let lastWidth = 0,
  lastHeight = 0;
let gesture = null,
  suppressClick = false,
  tabWidth = 1;
onMounted(async () => {
  try {
    const [core, tabs, types] = await Promise.all([
      import("@/vendor/liquid-glass-webgl/renderer/index.ts"),
      import("@/vendor/liquid-glass-webgl/catalog/build-bottom-tabs.ts"),
      import("@/vendor/liquid-glass-webgl/catalog/types.ts"),
    ]);
    if (disposed) return;
    buildBottomTabs = tabs.buildBottomTabs;
    defaults = types.DEFAULT_CATALOG_STATE;
    palette = types.LIGHT_PALETTE;
    canvas.value.getContext("webgl", {
      alpha: true,
      premultipliedAlpha: true,
      antialias: false,
      preserveDrawingBuffer: false,
      powerPreference: "low-power",
    });
    renderer = new core.LiquidGlassRenderer(canvas.value);
    renderer.quickToggles.perElementFbo = true;
    renderer.backgroundColor = [245 / 255, 245 / 255, 247 / 255];
    renderer.dpr = Math.min(
      window.devicePixelRatio || 1,
      renderer.isSoftwareRenderer ? 1.5 : 2,
    );
    output = createGlassOutput(renderer);
    const render = renderer.render.bind(renderer);
    renderer.render = () => {
      if (!renderer?.needsRedraw) return;
      output.begin();
      render();
      output.finish();
    };
    observer = new ResizeObserver(draw);
    observer.observe(host.value);
    draw();
  } catch {
    disposeRenderer();
    ready.value = false;
  }
});
watch(selected, () => {
  cancelPointer();
  selectTab(!reduced.matches);
});
reduced.addEventListener("change", motionChanged);
onUnmounted(() => {
  disposed = true;
  cancelPointer();
  observer?.disconnect();
  disposeRenderer();
  reduced.removeEventListener("change", motionChanged);
  window.removeEventListener("blur", cancelPointer);
});
window.addEventListener("blur", cancelPointer);
function draw() {
  if (
    !renderer ||
    !host.value ||
    host.value.clientWidth <= 0 ||
    host.value.clientHeight <= 0
  )
    return;
  const hostWidth = host.value.clientWidth;
  const hostHeight = host.value.clientHeight;
  if (hostWidth === lastWidth && hostHeight === lastHeight) return;
  lastWidth = hostWidth;
  lastHeight = hostHeight;
  cancelPointer();
  const scale = hostHeight / 64;
  host.value.style.padding = 4 * scale + "px";
  const width = hostWidth / scale + 72;
  Object.assign(canvas.value.style, {
    left: -36 * scale + "px",
    top: -24 * scale + "px",
    width: width * scale + "px",
    height: NAVIGATION_FRAME_HEIGHT * scale + "px",
    clipPath: "none",
  });
  renderer.resize(width, NAVIGATION_FRAME_HEIGHT);
  // Preserve the original backing, indicator, grouping and press physics.
  const result = buildBottomTabs(
    width,
    NAVIGATION_CATALOG_HEIGHT,
    () => {},
    { ...defaults, selectedTab: selected.value },
    () => {},
    { current: renderer },
    palette,
  );
  const elements = prepareNavigationElements(result.elements);
  tabWidth = elements.find((element) => element.isBottomTabIndicator)
    .isBottomTabIndicator.dragWidth;
  renderer.setElements(elements);
  selectTab(false);
  renderer.render();
  ready.value = true;
}
function selectTab(animate) {
  if (!renderer) return;
  renderer.setTabSelected("tabs3", selected.value, 3);
  const state = renderer.toggleStates.get("tabs3");
  if (!state) return;
  // Keep the upstream spring and press physics. Snap only for layout changes
  // or reduced motion; resizing must not replay a selection animation.
  if (!animate) {
    state.targetPress = state.pressProgress = state.pressVelocity = 0;
    state.targetScaleX = state.scaleX = state.targetScaleY = state.scaleY = 1;
    state.scaleXVelocity = state.scaleYVelocity = 0;
    state.fraction = state.targetFraction = selected.value;
    state.fractionVelocity = 0;
  }
  renderer.markGroupDirty("tabs3");
  renderer.requestRender();
}
function motionChanged() {
  if (reduced.matches) selectTab(false);
}
function startPointer(event) {
  suppressClick = false;
  if (
    !renderer ||
    event.button !== 0 ||
    !event.isPrimary ||
    event.ctrlKey ||
    event.metaKey ||
    event.altKey ||
    event.shiftKey
  )
    return;
  const scale = host.value.clientHeight / 64;
  gesture = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startY: event.clientY,
    startIndex: renderer.getTabTarget("tabs3"),
    scale,
    active: false,
  };
}
function movePointer(event) {
  if (!renderer || !gesture || event.pointerId !== gesture.pointerId) return;
  const dx = event.clientX - gesture.startX;
  if (!gesture.active) {
    if (
      Math.abs(dx) < 4 ||
      Math.abs(dx) < Math.abs(event.clientY - gesture.startY)
    )
      return;
    gesture.active = true;
    dragging.value = true;
    host.value.setPointerCapture(event.pointerId);
    renderer.beginTabDrag("tabs3", gesture.startIndex, items.length);
  }
  event.preventDefault();
  renderer.dragTab(
    "tabs3",
    gesture.startIndex,
    event.clientX / gesture.scale,
    gesture.startX / gesture.scale,
    tabWidth,
    items.length,
  );
  if (reduced.matches) snapDrag(renderer.getTabTarget("tabs3"));
}
function endPointer(event) {
  if (!gesture || event.pointerId !== gesture.pointerId) return;
  const current = gesture;
  if (current.active) movePointer(event);
  gesture = null;
  dragging.value = false;
  if (current.active && renderer) {
    suppressClick = true;
    const index = renderer.endTabDrag("tabs3", items.length);
    if (reduced.matches) snapDrag(index);
    if (host.value.hasPointerCapture(current.pointerId))
      host.value.releasePointerCapture(current.pointerId);
    router.push(items[index].to);
  }
}
function cancelPointer(event) {
  if (
    !gesture ||
    (event?.pointerId !== undefined && event.pointerId !== gesture.pointerId)
  )
    return;
  const current = gesture;
  gesture = null;
  dragging.value = false;
  if (current.active && renderer) {
    suppressClick = true;
    renderer.endTabDrag("tabs3", items.length);
    selectTab(!reduced.matches);
    if (host.value?.hasPointerCapture(current.pointerId))
      host.value.releasePointerCapture(current.pointerId);
  }
}
function guardClick(event) {
  if (suppressClick && event.detail !== 0) {
    event.preventDefault();
    event.stopPropagation();
  }
  suppressClick = false;
}
function snapDrag(index) {
  const state = renderer.toggleStates.get("tabs3");
  state.fraction = index;
  state.fractionVelocity = 0;
  state.targetPress = state.pressProgress = state.pressVelocity = 0;
  state.targetScaleX = state.scaleX = state.targetScaleY = state.scaleY = 1;
  state.scaleXVelocity =
    state.scaleYVelocity =
    state.velocity =
    state.targetVelocity =
      0;
  renderer.markGroupDirty("tabs3");
  renderer.requestRender();
}
function loseContext(event) {
  event.preventDefault();
  ready.value = false;
  cancelPointer();
  observer?.disconnect();
  disposeRenderer(false);
}
function disposeRenderer(release = true) {
  const current = renderer;
  renderer = undefined;
  output?.dispose();
  output = undefined;
  current?.dispose();
  if (release) current?.gl.getExtension("WEBGL_lose_context")?.loseContext();
}
</script>

<style scoped>
.liquid-nav {
  touch-action: pan-y;
  user-select: none;
}
.liquid-nav a {
  -webkit-user-drag: none;
}
.liquid-nav.is-dragging,
.liquid-nav.is-dragging a {
  cursor: grabbing;
}
</style>
