<template>
  <div class="select-field" :class="{ 'is-disabled': disabled }">
    <label :id="labelId" :for="fieldId">{{ label }}</label>
    <GlassContainer
      :id="fieldId"
      ref="trigger"
      as="button"
      type="button"
      role="combobox"
      class="select-trigger"
      :disabled="disabled"
      :aria-labelledby="labelId"
      :aria-expanded="open"
      :aria-controls="menuId"
      :aria-activedescendant="open && active >= 0 ? optionId(active) : undefined"
      aria-haspopup="listbox"
      :title="selected?.description"
      @click="toggle"
      @keydown="triggerKeydown"
    >
      <span class="select-value">{{ selected?.label || "请选择" }}</span>
      <svg class="select-chevron" viewBox="0 0 16 16" aria-hidden="true">
        <path d="m5 6 3-3 3 3M5 10l3 3 3-3" />
      </svg>
    </GlassContainer>
    <PopupPanel
      :id="menuId"
      :open="open"
      :anchor="trigger"
      :label="label"
      role="listbox"
      panel-class="select-options"
      focus-selector='[data-active="true"]'
      match-width
      keep-anchor-focus
      align="start"
      @dismiss="close"
    >
      <div @keydown="optionKeydown">
        <button
          v-for="(option, index) in options"
          :id="optionId(index)"
          :key="option.value"
          type="button"
          role="option"
          class="ui-menu-item select-option"
          tabindex="-1"
          :disabled="option.disabled"
          :aria-selected="option.value === modelValue"
          :aria-describedby="option.description ? optionId(index) + '-help' : undefined"
          :data-active="index === active"
          @pointerdown.prevent
          @pointermove="!option.disabled && (active = index)"
          @click="choose(index)"
        >
          <svg class="select-check" viewBox="0 0 16 16" aria-hidden="true">
            <path v-if="option.value === modelValue" d="m3.5 8 3 3 6-6" />
          </svg>
          <span class="select-option-copy">
            <span>{{ option.label }}</span>
            <small v-if="option.description" :id="optionId(index) + '-help'">
              {{ option.description }}
            </small>
          </span>
        </button>
      </div>
    </PopupPanel>
  </div>
</template>
<script setup>
import { computed, nextTick, ref, useId, watch } from "vue";
import GlassContainer from "./GlassContainer.vue";
import PopupPanel from "./PopupPanel.vue";

const props = defineProps({
  id: String,
  label: { type: String, required: true },
  modelValue: { type: String, default: "" },
  options: { type: Array, required: true },
  disabled: Boolean,
});
const emit = defineEmits(["update:modelValue", "change"]);
const uniqueId = useId();
const fieldId = computed(() => props.id || "select-" + uniqueId);
const labelId = computed(() => fieldId.value + "-label");
const menuId = computed(() => fieldId.value + "-options");
const optionId = index => fieldId.value + "-option-" + index;
const selected = computed(() =>
  props.options.find(option => option.value === props.modelValue));
const enabled = computed(() =>
  props.options.flatMap((option, index) => option.disabled ? [] : [index]));
const trigger = ref(null), open = ref(false), active = ref(-1);
let search = "", searchAt = 0;

function show(edge) {
  if (props.disabled || !enabled.value.length) return;
  active.value = edge === "first" ? enabled.value[0] : edge === "last" ?
    enabled.value.at(-1) : props.options.findIndex(option =>
      option.value === props.modelValue && !option.disabled);
  if (active.value < 0) active.value = enabled.value[0];
  open.value = true;
}
function close() { open.value = false; search = ""; }
function toggle() { open.value ? close() : show(); }
async function focusActive() {
  await nextTick();
  if (!open.value) return;
  const option = document.getElementById(optionId(active.value));
  option?.scrollIntoView({ block: "nearest" });
}
function choose(index) {
  const option = props.options[index];
  if (props.disabled || !option || option.disabled) return;
  close();
  trigger.value?.$el.focus({ preventScroll: true });
  if (option.value !== props.modelValue) {
    emit("update:modelValue", option.value);
    emit("change", option.value);
  }
}
function move(key) {
  const current = enabled.value.indexOf(active.value);
  if (key === "Home") active.value = enabled.value[0];
  else if (key === "End") active.value = enabled.value.at(-1);
  else active.value = enabled.value[Math.max(0, Math.min(
    enabled.value.length - 1, current + (key === "ArrowDown" ? 1 : -1)))];
  focusActive();
}
function typeahead(event) {
  if (event.key.length !== 1 || event.ctrlKey || event.altKey || event.metaKey || event.isComposing)
    return false;
  event.preventDefault();
  const now = performance.now(), letter = event.key.toLowerCase();
  search = now - searchAt > 650 ? letter : search + letter;
  searchAt = now;
  const repeated = [...search].every(character => character === search[0]);
  const query = repeated ? search[0] : search;
  const current = enabled.value.indexOf(active.value);
  const ordered = repeated ? [...enabled.value.slice(current + 1),
    ...enabled.value.slice(0, current + 1)] : enabled.value;
  const match = ordered.find(index =>
    [props.options[index].label, props.options[index].value].some(text =>
      String(text).toLowerCase().startsWith(query)));
  if (match !== undefined) { active.value = match; focusActive(); }
  return true;
}
function triggerKeydown(event) {
  if (props.disabled) return;
  if (open.value) {
    if (event.key === "Escape") {
      event.preventDefault();
      close();
    } else optionKeydown(event);
    return;
  }
  if (["ArrowDown", "ArrowUp", "Enter", " ", "Home", "End"].includes(event.key)) {
    event.preventDefault();
    show(event.key === "Home" ? "first" : event.key === "End" ? "last" : undefined);
  } else if (event.key.length === 1 && !event.ctrlKey && !event.altKey && !event.metaKey) {
    show();
    typeahead(event);
  }
}
function optionKeydown(event) {
  if (event.key === "Tab") {
    choose(active.value);
  } else if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
    event.preventDefault();
    move(event.key);
  } else if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    choose(active.value);
  } else typeahead(event);
}
watch(() => [props.disabled, props.options], () => {
  if (props.disabled || !enabled.value.length) close();
  else if (open.value && !enabled.value.includes(active.value)) {
    active.value = enabled.value[0];
    focusActive();
  }
});
</script>
