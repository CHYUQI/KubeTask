<template>
  <GlassButton
    ref="trigger"
    :label="label"
    :disabled="unavailable"
    aria-haspopup="menu"
    :aria-expanded="open"
    :aria-controls="menuId"
    @click="toggle"
    @keydown="triggerKeydown"
  />
  <PopupPanel
    :id="menuId"
    :open="open"
    :anchor="trigger"
    :label="label"
    :focus-last="focusLast"
    role="menu"
    panel-class="action-menu-items"
    @dismiss="open = false"
  >
    <div @keydown="menuKeydown">
      <template v-for="(item, index) in items" :key="item.label">
        <div v-if="item.danger && index" class="ui-menu-separator" role="separator" />
        <button
          type="button"
          role="menuitem"
          tabindex="-1"
          class="ui-menu-item"
          :data-variant="item.danger ? 'destructive' : undefined"
          :disabled="item.disabled"
          @click="choose(item)"
        >{{ item.label }}</button>
      </template>
    </div>
  </PopupPanel>
</template>
<script setup>
import { computed, ref, useId, watch } from "vue";
import GlassButton from "./GlassButton.vue";
import PopupPanel from "./PopupPanel.vue";
const props = defineProps({
  label: { type: String, default: "更多" },
  items: { type: Array, required: true },
});
const trigger = ref(null),
  open = ref(false),
  focusLast = ref(false);
const menuId = `actions-${useId()}`;
const unavailable = computed(() => props.items.every((item) => item.disabled));
watch(unavailable, (disabled) => {
  if (disabled) open.value = false;
});
function toggle() {
  focusLast.value = false;
  open.value = !open.value;
}
function triggerKeydown(event) {
  if (!["ArrowDown", "ArrowUp"].includes(event.key) || unavailable.value) return;
  event.preventDefault();
  focusLast.value = event.key === "ArrowUp";
  open.value = true;
}
function menuKeydown(event) {
  if (event.key === "Tab") {
    // Resume the document's tab order at the invoking button.
    trigger.value?.$el.focus({ preventScroll: true });
    open.value = false;
    return;
  }
  const buttons = [...event.currentTarget.querySelectorAll('button:not(:disabled)')];
  const index = buttons.indexOf(document.activeElement);
  let next;
  if (event.key === "Home") next = 0;
  else if (event.key === "End") next = buttons.length - 1;
  else if (event.key === "ArrowDown") next = (index + 1) % buttons.length;
  else if (event.key === "ArrowUp") next = (index - 1 + buttons.length) % buttons.length;
  else if (event.key.length === 1 && !event.ctrlKey && !event.altKey && !event.metaKey) {
    const ordered = [...buttons.slice(index + 1), ...buttons.slice(0, index + 1)];
    const match = ordered.find((button) =>
      button.textContent.trim().toLowerCase().startsWith(event.key.toLowerCase()),
    );
    next = buttons.indexOf(match);
  }
  if (next >= 0) {
    event.preventDefault();
    buttons[next]?.focus({ preventScroll: true });
  }
}
function choose(item) {
  if (item.disabled) return;
  open.value = false;
  trigger.value?.$el.focus({ preventScroll: true });
  item.action();
}
</script>
