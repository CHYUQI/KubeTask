<template>
  <svg
    class="app-icon"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.65"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path
      v-for="(path, index) in paths[name] || paths.box"
      :key="index"
      :d="path"
    />
  </svg>
</template>
<script setup>
defineProps({ name: { type: String, default: "box" } });
const paths = {
  grid: ["M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z"],
  box: ["m12 3 9 5-9 5-9-5 9-5Z", "M3 8v9l9 5 9-5V8 M12 13v9 M7.5 5.5l9 5"],
  workflow: [
    "M3 3h6v6H3z M15 15h6v6h-6z M3 15h6v6H3z",
    "M6 9v6 M9 6h6a3 3 0 0 1 3 3v6",
  ],
  plus: ["M12 5v14 M5 12h14"],
  arrow: ["M5 12h14 m-5-5 5 5-5 5"],
  chevron: ["m9 5 7 7-7 7"],
  refresh: [
    "M20 7v5h-5",
    "M4 17v-5h5",
    "M6.1 6.1a8.3 8.3 0 0 1 13.4 3.4 M4.5 14.5a8.3 8.3 0 0 0 13.4 3.4",
  ],
  play: ["m9 5 11 7-11 7V5Z"],
  check: ["m5 12 4 4L19 6"],
  close: ["m6 6 12 12 M6 18 18 6"],
  pause: ["M8 5v14 M16 5v14"],
  clock: ["M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z", "M12 7v5l3 2"],
  logs: ["M5 3h14v18H5z", "M9 7h6 M9 11h6 M9 15h3"],
  trash: ["M3 6h18 M9 6V3h6v3 M5 6l1 15h12l1-15 M10 10v7 M14 10v7"],
  shield: [
    "m12 3 8 3v5c0 5-8 10-8 10S4 16 4 11V6l8-3Z",
    "m8.5 11 2.5 2.5 4.5-5",
  ],
  command: [
    "M8 9H6a3 3 0 1 1 3-3v12a3 3 0 1 1-3-3h12a3 3 0 1 1-3 3V6a3 3 0 1 1 3 3H8Z",
  ],
  activity: ["M3 12h4l3-8 4 16 3-8h4"],
  search: ["M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0Z", "m15 15 6 6"],
  spark: ["m12 3 2.2 6.8L21 12l-6.8 2.2L12 21l-2.2-6.8L3 12l6.8-2.2L12 3Z"],
  cloud: [
    "M6 18a5 5 0 0 1-1-9.9A7 7 0 0 1 18.5 8a5 5 0 0 1-.5 10",
    "M9 16v5 M15 16v5",
  ],
  code: ["m8 7-5 5 5 5 m8-10 5 5-5 5 M14 4l-4 16"],
};
</script>
