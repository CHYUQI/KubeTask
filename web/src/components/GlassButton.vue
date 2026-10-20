<template>
  <component
    :is="to ? RouterLink : 'button'"
    ref="host"
    :to="to"
    :type="to ? undefined : type"
    :disabled="to ? undefined : disabled"
    class="glass-button"
    :class="[tone, { ready }]"
    @pointerdown="press(true, $event)"
    @pointerup="press(false)"
    @pointerleave="press(false)"
    @pointercancel="press(false)"
    @blur="press(false)"
    @keydown.space="press(true)"
    @keyup.space="press(false)"
  >
    <canvas
      ref="canvas"
      aria-hidden="true"
      @webglcontextlost="loseContext"
    /><span>{{ label }}</span>
  </component>
</template>
<script setup>
import { nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
const props = defineProps({
  label: { type: String, required: true },
  tone: { type: String, default: "neutral" },
  to: [String, Object],
  type: { type: String, default: "button" },
  disabled: Boolean,
});
const host = ref(null),
  canvas = ref(null),
  ready = ref(false);
const padding = 16;
const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
let renderer,
  buildButtons,
  defaults,
  palette,
  observer,
  disposed = false;
const elementId = () =>
  props.tone === "accent" ? "btn-tinted-blue" : "btn-surface";
const node = () => host.value?.$el || host.value;
onMounted(async () => {
  try {
    const [core, buttons, types] = await Promise.all([
      import("@/vendor/liquid-glass-webgl/renderer/index.ts"),
      import("@/vendor/liquid-glass-webgl/catalog/build-buttons.ts"),
      import("@/vendor/liquid-glass-webgl/catalog/types.ts"),
    ]);
    if (disposed) return;
    buildButtons = buttons.buildButtons;
    defaults = types.DEFAULT_CATALOG_STATE;
    palette = types.LIGHT_PALETTE;
    renderer = new core.LiquidGlassRenderer(canvas.value);
    renderer.backgroundColor = backgroundColor(node());
    renderer.dpr = Math.min(window.devicePixelRatio || 1, 2);
    observer = new ResizeObserver(draw);
    observer.observe(node());
    draw();
  } catch {
    disposeRenderer();
    ready.value = false;
  }
});
watch(
  () => [props.label, props.tone],
  async () => {
    await nextTick();
    draw();
  },
);
onUnmounted(() => {
  disposed = true;
  observer?.disconnect();
  disposeRenderer();
});
function draw() {
  const bounds = node()?.getBoundingClientRect();
  if (!renderer || !bounds || bounds.width <= 0 || bounds.height <= 0) return;
  const scale = bounds.height / 48;
  const width = bounds.width / scale;
  const frameWidth = width + padding * 2;
  const frameHeight = 48 + padding * 2;
  Object.assign(canvas.value.style, {
    left: -padding * scale + "px",
    top: -padding * scale + "px",
    width: frameWidth * scale + "px",
    height: frameHeight * scale + "px",
  });
  renderer.resize(frameWidth, frameHeight);
  // Select an actual upstream button component. Rebind its label and bounds;
  // Refraction, highlights and tint are the repository's own.
  const element = buildButtons(
    frameWidth,
    frameHeight,
    () => {},
    defaults,
    palette,
  ).elements.find((element) => element.id === elementId());
  element.rect = { x: padding, y: padding, w: width, h: 48 };
  element.cornerRadius = 24;
  // Keep text in the DOM: it stays sharp, accessible and visible even when
  // WebGL is loading or unavailable. The upstream component paints material.
  element.label = "";
  renderer.setElements([element]);
  renderer.render();
  ready.value = true;
}
function backgroundColor(element) {
  for (
    let parent = element?.parentElement;
    parent;
    parent = parent.parentElement
  ) {
    const color = getComputedStyle(parent).backgroundColor;
    const channels = color.match(/[\d.]+/g)?.map(Number);
    if (channels?.length >= 3 && (channels.length === 3 || channels[3] === 1))
      return channels.slice(0, 3).map((channel) => channel / 255);
  }
  return [245 / 255, 245 / 255, 247 / 255];
}
function loseContext(event) {
  event.preventDefault();
  ready.value = false;
  observer?.disconnect();
  disposeRenderer(false);
}
function disposeRenderer(release = true) {
  const current = renderer;
  renderer = undefined;
  current?.dispose();
  if (release) current?.gl.getExtension("WEBGL_lose_context")?.loseContext();
}
function press(pressed, event) {
  if (!renderer || (pressed && (props.disabled || reduced.matches))) return;
  const bounds = node()?.getBoundingClientRect();
  const scale = bounds?.height / 48;
  const position =
    event && scale > 0
      ? {
          x: (event.clientX - bounds.left) / scale + padding,
          y: (event.clientY - bounds.top) / scale + padding,
        }
      : undefined;
  renderer.setPressed(elementId(), pressed, position);
}
</script>
