<template>
  <div class="select-field" :class="{ 'is-disabled': disabled }">
    <label :id="labelId" :for="fieldId">{{ label }}</label>
    <button
      :id="fieldId"
      ref="trigger"
      type="button"
      role="combobox"
      class="select-trigger"
      :class="{ 'is-open': open }"
      :disabled="disabled"
      :aria-labelledby="labelId"
      :aria-expanded="open"
      :aria-controls="menuId"
      :aria-activedescendant="
        open && active >= 0 ? optionId(active) : undefined
      "
      aria-haspopup="listbox"
      @click="toggle"
      @keydown="onKeydown"
      @blur="close"
    >
      <span class="select-value">{{ selected?.label || "请选择" }}</span>
      <svg class="select-chevron" viewBox="0 0 16 16" aria-hidden="true">
        <path d="m5 6 3-3 3 3M5 10l3 3 3-3" />
      </svg>
    </button>
    <Teleport to="body">
      <Transition name="select-menu">
        <ul
          v-if="open"
          :id="menuId"
          ref="menu"
          role="listbox"
          :aria-labelledby="labelId"
          class="select-options"
          :style="menuStyle"
        >
          <li
            v-for="(option, index) in options"
            :id="optionId(index)"
            :key="option.value"
            role="option"
            :aria-selected="option.value === modelValue"
            :aria-disabled="option.disabled || undefined"
            :class="{
              'is-active': index === active,
              'is-selected': option.value === modelValue,
              'is-unavailable': option.disabled,
            }"
            @pointerdown.prevent
            @pointermove="!option.disabled && (active = index)"
            @click="choose(index)"
          >
            <svg class="select-check" viewBox="0 0 16 16" aria-hidden="true">
              <path v-if="option.value === modelValue" d="m3.5 8 3 3 6-6" />
            </svg>
            <span class="select-option-copy"
              ><span>{{ option.label }}</span
              ><small v-if="option.description">{{
                option.description
              }}</small></span
            >
          </li>
        </ul>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup>
import {
  computed,
  nextTick,
  onMounted,
  onUnmounted,
  ref,
  useId,
  watch,
} from "vue";
const props = defineProps({
  id: String,
  label: { type: String, required: true },
  modelValue: { type: String, default: "" },
  options: { type: Array, required: true },
  disabled: Boolean,
});
const emit = defineEmits(["update:modelValue", "change"]);
const uniqueId = useId();
const fieldId = computed(() => props.id || `select-${uniqueId}`);
const labelId = computed(() => `${fieldId.value}-label`);
const menuId = computed(() => `${fieldId.value}-options`);
const optionId = (index) => `${fieldId.value}-option-${index}`;
const trigger = ref(null),
  menu = ref(null),
  open = ref(false),
  active = ref(-1),
  menuStyle = ref({});
const selected = computed(() =>
  props.options.find((option) => option.value === props.modelValue),
);
const enabled = computed(() =>
  props.options.flatMap((option, index) => (option.disabled ? [] : [index])),
);
let search = "",
  searchAt = 0;

async function show() {
  if (props.disabled || !enabled.value.length) return;
  active.value = props.options.findIndex(
    (option) => option.value === props.modelValue && !option.disabled,
  );
  if (active.value < 0) active.value = enabled.value[0];
  open.value = true;
  await nextTick();
  if (!open.value) return;
  await position();
  reveal();
}
function toggle() {
  open.value ? close() : show();
}
function close() {
  open.value = false;
  search = "";
}
function choose(index) {
  const option = props.options[index];
  if (!option || option.disabled || props.disabled) return;
  close();
  trigger.value?.focus({ preventScroll: true });
  if (option.value !== props.modelValue) {
    emit("update:modelValue", option.value);
    emit("change", option.value);
  }
}
async function position() {
  const rect = trigger.value?.getBoundingClientRect();
  if (!rect || !menu.value) return;
  const viewportWidth = document.documentElement.clientWidth;
  const viewportHeight = window.innerHeight;
  const width = Math.min(Math.max(rect.width, 240), viewportWidth - 24);
  const left = Math.max(12, Math.min(rect.left, viewportWidth - width - 12));
  menuStyle.value = {
    width: `${width}px`,
    left: `${left}px`,
    top: `${rect.bottom + 8}px`,
    visibility: "hidden",
  };
  await nextTick();
  if (!open.value || !menu.value) return;
  const below = viewportHeight - rect.bottom - 16;
  const above = rect.top - 16;
  const height = Math.min(menu.value.scrollHeight, 320);
  const upwards = below < height && above > below;
  const maxHeight = Math.max(80, Math.min(320, upwards ? above : below));
  menuStyle.value = {
    width: `${width}px`,
    left: `${left}px`,
    top: `${upwards ? rect.top - Math.min(height, maxHeight) - 8 : rect.bottom + 8}px`,
    maxHeight: `${maxHeight}px`,
  };
}
async function reveal() {
  await nextTick();
  if (!open.value) return;
  const option = document.getElementById(optionId(active.value));
  if (!option || !menu.value) return;
  if (option.offsetTop < menu.value.scrollTop)
    menu.value.scrollTop = option.offsetTop;
  else if (
    option.offsetTop + option.offsetHeight >
    menu.value.scrollTop + menu.value.clientHeight
  )
    menu.value.scrollTop =
      option.offsetTop + option.offsetHeight - menu.value.clientHeight;
}
function move(delta) {
  const current = enabled.value.indexOf(active.value);
  active.value =
    enabled.value[
      Math.max(0, Math.min(enabled.value.length - 1, current + delta))
    ];
  reveal();
}
async function onKeydown(event) {
  const { key } = event;
  if (key === "Escape") {
    event.preventDefault();
    close();
    return;
  }
  if (key === "Tab") {
    if (open.value) choose(active.value);
    return;
  }
  if (["Enter", " "].includes(key)) {
    event.preventDefault();
    if (open.value) choose(active.value);
    else await show();
    return;
  }
  if (
    ["ArrowDown", "ArrowUp", "Home", "End", "PageDown", "PageUp"].includes(key)
  ) {
    event.preventDefault();
    const wasOpen = open.value;
    if (!wasOpen) await show();
    if (key === "Home") active.value = enabled.value[0];
    else if (key === "End") active.value = enabled.value.at(-1);
    else if (key === "ArrowUp" && event.altKey && wasOpen) {
      choose(active.value);
      return;
    } else if (wasOpen)
      move(
        key === "ArrowDown"
          ? 1
          : key === "ArrowUp"
            ? -1
            : key === "PageDown"
              ? 10
              : -10,
      );
    reveal();
    return;
  }
  if (key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
    event.preventDefault();
    if (!open.value) await show();
    const now = performance.now();
    search =
      now - searchAt > 650 ? key.toLowerCase() : search + key.toLowerCase();
    searchAt = now;
    const repeated = [...search].every((character) => character === search[0]);
    const query = repeated ? search[0] : search;
    const current = enabled.value.indexOf(active.value);
    const ordered = repeated
      ? [
          ...enabled.value.slice(current + 1),
          ...enabled.value.slice(0, current + 1),
        ]
      : enabled.value;
    const match = ordered.find((index) =>
      [props.options[index].label, props.options[index].value].some((text) =>
        String(text).toLowerCase().startsWith(query),
      ),
    );
    if (match !== undefined) {
      active.value = match;
      reveal();
    }
  }
}
function outside(event) {
  if (
    !trigger.value?.contains(event.target) &&
    !menu.value?.contains(event.target)
  )
    close();
}
function onScroll(event) {
  if (!menu.value?.contains(event.target)) close();
}
watch(
  () => props.disabled,
  (value) => {
    if (value) close();
  },
);
onMounted(() => {
  document.addEventListener("pointerdown", outside, true);
  window.addEventListener("resize", close);
  window.addEventListener("scroll", onScroll, true);
});
onUnmounted(() => {
  document.removeEventListener("pointerdown", outside, true);
  window.removeEventListener("resize", close);
  window.removeEventListener("scroll", onScroll, true);
});
</script>
