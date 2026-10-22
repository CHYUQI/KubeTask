<template>
  <GlassContainer class="no-cluster">
    <h2>未连接 Kubernetes 集群</h2>
    <p>{{ message }}</p>
  </GlassContainer>
</template>

<script setup>
import GlassContainer from "@/components/GlassContainer.vue";
defineProps({
  message: { type: String, default: "无法连接到后端服务" },
});
</script>
