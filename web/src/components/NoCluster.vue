<template>
  <div class="no-cluster">
    <div class="no-cluster-icon">⚡</div>
    <h2>未连接 Kubernetes 集群</h2>
    <p>{{ message }}</p>
    <p class="hint">请确保 KubeTask 后端已启动且连接到了 K8s 集群<br/>或使用 standalone 模式运行</p>
  </div>
</template>

<script setup>
defineProps({
  message: { type: String, default: '无法连接到后端服务' }
})
</script>

<style scoped>
.no-cluster { text-align: center; padding: 60px 20px; }
.no-cluster-icon { font-size: 48px; margin-bottom: 12px; }
.no-cluster h2 { font-size: 20px; margin-bottom: 8px; color: #334155; }
.no-cluster p { color: #64748b; font-size: 14px; }
.no-cluster .hint { margin-top: 16px; color: #94a3b8; font-size: 13px; line-height: 1.6; }
</style>
