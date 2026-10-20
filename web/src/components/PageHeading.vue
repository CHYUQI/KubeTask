<template>
  <header class="page-header">
    <router-link v-if="backTo" :to="backTo" class="back-link">
      <span aria-hidden="true">‹</span>{{ backLabel }}
    </router-link>
    <div class="page-heading">
      <div class="page-heading-title">
        <h1>{{ title }}</h1>
        <p v-if="description" class="page-description">{{ description }}</p>
      </div>
      <div v-if="$slots.default" class="page-heading-actions"><slot /></div>
    </div>
  </header>
</template>
<script setup>
defineProps({
  title: String,
  description: String,
  backTo: [String, Object],
  backLabel: { type: String, default: "返回" },
});
</script>
