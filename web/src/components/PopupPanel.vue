<template>
  <Teleport to="body">
    <GlassContainer
      v-if="open"
      :id="id"
      ref="panel"
      class="ui-popup"
      :class="panelClass"
      :style="position"
      :role="role"
      :aria-label="label"
      tabindex="-1"
      @keydown.esc.stop.prevent="emit('dismiss')"
    >
      <div ref="content" class="popup-content"><slot /></div>
    </GlassContainer>
  </Teleport>
</template>
<script setup>
import { nextTick, onUnmounted, ref, watch } from "vue";
import GlassContainer from "./GlassContainer.vue";

const props = defineProps({
  id: { type: String, required: true },
  open: Boolean,
  anchor: Object,
  label: String,
  role: { type: String, default: "dialog" },
  panelClass: String,
  focusLast: Boolean,
  focusSelector: String,
  keepAnchorFocus: Boolean,
  matchWidth: Boolean,
  align: { type: String, default: "end" },
});
const emit = defineEmits(["dismiss"]);
const panel = ref(null), content = ref(null);
const position = ref({ visibility: "hidden" });
let observer, frame = 0;
const anchorNode = () => props.anchor?.$el || props.anchor;
const panelNode = () => panel.value?.$el;

function place() {
  frame = 0;
  const anchor = anchorNode(), element = panelNode();
  if (!props.open || !element || !anchor || !content.value) return;
  const rect = anchor.getBoundingClientRect();
  const width = document.documentElement.clientWidth;
  const height = window.visualViewport?.height || window.innerHeight;
  const popupWidth = props.matchWidth ? Math.min(rect.width, width - 24) : null;
  if (popupWidth !== null) element.style.width = popupWidth + "px";
  // Measure content, never the decorative canvas and its shadow overscan.
  const naturalHeight = content.value.scrollHeight;
  const below = height - rect.bottom - 16, above = rect.top - 16;
  const upwards = naturalHeight > below && above > below;
  const maxHeight = Math.max(0, upwards ? above : below);
  const panelHeight = Math.min(naturalHeight, maxHeight);
  const panelWidth = element.offsetWidth;
  const preferredLeft = props.align === "start" ? rect.left : rect.right - panelWidth;
  position.value = {
    width: popupWidth === null ? undefined : popupWidth + "px",
    left: Math.max(12, Math.min(preferredLeft, width - panelWidth - 12)) + "px",
    top: Math.max(12, upwards ? rect.top - panelHeight - 4 : rect.bottom + 4) + "px",
    maxHeight: maxHeight + "px",
  };
}
function schedule(event) {
  if (event?.type === "scroll" && content.value?.contains(event.target)) return;
  if (!frame) frame = requestAnimationFrame(place);
}
function outside(event) {
  if (!panelNode()?.contains(event.target) && !anchorNode()?.contains(event.target))
    emit("dismiss");
}
function stop() {
  observer?.disconnect();
  cancelAnimationFrame(frame);
  frame = 0;
  document.removeEventListener("pointerdown", outside, true);
  document.removeEventListener("focusin", outside);
  window.removeEventListener("resize", schedule);
  window.removeEventListener("scroll", schedule, true);
  window.visualViewport?.removeEventListener("resize", schedule);
}
watch(() => props.open, async (open, _previous, onCleanup) => {
  const restoreFocus = panelNode()?.contains(document.activeElement);
  stop();
  if (!open) {
    position.value = { visibility: "hidden" };
    if (restoreFocus) anchorNode()?.focus({ preventScroll: true });
    return;
  }
  let cancelled = false;
  onCleanup(() => { cancelled = true; stop(); });
  await nextTick();
  if (cancelled || !props.open || !panelNode()) return;
  place();
  await nextTick();
  if (cancelled || !props.open || !panelNode()) return;
  const element = panelNode();
  const items = element.querySelectorAll('[role="menuitem"]:not(:disabled)');
  const focus = props.focusSelector ? element.querySelector(props.focusSelector) :
    props.role === "menu" ? (props.focusLast ? items[items.length - 1] : items[0]) :
    element.querySelector("[autofocus]");
  if (props.keepAnchorFocus) {
    anchorNode()?.focus({ preventScroll: true });
    focus?.scrollIntoView({ block: "nearest" });
  } else (focus || element).focus({ preventScroll: true });
  document.addEventListener("pointerdown", outside, true);
  document.addEventListener("focusin", outside);
  window.addEventListener("resize", schedule);
  window.addEventListener("scroll", schedule, true);
  window.visualViewport?.addEventListener("resize", schedule);
  observer = new ResizeObserver(schedule);
  observer.observe(content.value);
  if (anchorNode()) observer.observe(anchorNode());
}, { immediate: true });
onUnmounted(stop);
</script>
