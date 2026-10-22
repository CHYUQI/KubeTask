<template>
  <component
    :is="to ? RouterLink : 'button'"
    ref="host"
    :to="to"
    :type="to ? undefined : type"
    :disabled="to ? undefined : disabled"
    :aria-label="label"
    class="glass-button"
    :class="[tone, { ready }]"
    @pointerdown="pointerDown"
    @pointermove="pointerMove"
    @pointerup="release"
    @pointerleave="release"
    @pointercancel="release"
    @blur="release"
    @keydown="keyDown"
    @keyup="keyUp"
  >
    <canvas ref="canvas" aria-hidden="true" />
    <span aria-hidden="true">{{ label }}</span>
  </component>
</template>
<script setup>
import { nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { registerButtonSurface } from "./button-surfaces.js";
import { buttonPointerPosition } from "./button-material.js";
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
const reduced = window.matchMedia("(prefers-reduced-motion: reduce)");
let surface, observer, pointerId, pressed = false;
const node = () => host.value?.$el || host.value;
onMounted(() => {
  surface = registerButtonSurface(canvas.value, describe, (value) => {
    ready.value = value;
  });
  observer = new ResizeObserver(() => surface.update());
  observer.observe(node());
  window.addEventListener("blur", release);
  reduced.addEventListener("change", release);
});
watch(
  () => [props.label, props.tone],
  async () => {
    await nextTick();
    surface?.update();
  },
);
watch(
  () => props.disabled,
  (disabled) => {
    if (disabled) release();
  },
);
onUnmounted(() => {
  observer?.disconnect();
  window.removeEventListener("blur", release);
  reduced.removeEventListener("change", release);
  surface?.dispose();
});
function describe() {
  const element = node();
  const { width, height } = element.getBoundingClientRect();
  return {
    width,
    height,
    label: props.label,
    tone: props.tone,
    fontSize: parseFloat(getComputedStyle(element).fontSize),
    background: backgroundColor(element),
  };
}
function backgroundColor(element) {
  for (
    let parent = element?.parentElement;
    parent;
    parent = parent.parentElement
  ) {
    const channels = getComputedStyle(parent)
      .backgroundColor.match(/[\d.]+/g)
      ?.map(Number);
    if (channels?.length >= 3 && (channels.length === 3 || channels[3] === 1))
      return channels.slice(0, 3).map((channel) => channel / 255);
  }
  return [245 / 255, 245 / 255, 247 / 255];
}
function position(event) {
  return buttonPointerPosition(node().getBoundingClientRect(), event);
}
function pointerDown(event) {
  if (
    props.disabled ||
    reduced.matches ||
    event.button !== 0 ||
    !event.isPrimary
  )
    return;
  pointerId = event.pointerId;
  pressed = true;
  surface?.press(true, position(event));
}
function pointerMove(event) {
  if (event.pointerId === pointerId) surface?.drag(position(event));
}
function release() {
  if (!pressed) return;
  pressed = false;
  pointerId = undefined;
  surface?.press(false);
}
function keyDown(event) {
  if (
    (event.key === " " || event.key === "Enter") &&
    !event.repeat &&
    !props.disabled &&
    !reduced.matches
  )
    {
      pressed = true;
      surface?.press(true);
    }
}
function keyUp(event) {
  if (event.key === " " || event.key === "Enter") release();
}
</script>
