<template>
  <component
    :is="to ? RouterLink : as"
    ref="host"
    :to="to"
    class="liquid-container"
    :class="{ ready }"
  >
    <canvas ref="canvas" aria-hidden="true" />
    <slot />
  </component>
</template>
<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { registerContainerSurface } from "./container-surfaces.js";

defineProps({
  as: { type: String, default: "div" },
  to: [String, Object],
});
const host = ref(null), canvas = ref(null), ready = ref(false);
let surface, observer;
onMounted(() => {
  const element = host.value?.$el || host.value;
  surface = registerContainerSurface(element, canvas.value, (value) => {
    ready.value = value;
  });
  observer = new ResizeObserver(surface.update);
  observer.observe(element);
});
onUnmounted(() => {
  observer?.disconnect();
  surface?.dispose();
});
</script>
