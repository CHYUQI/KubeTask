<template>
  <div class="dashboard">
    <PageHeading title="总览">
      <GlassButton
        :label="loading ? '刷新中…' : '刷新'"
        :disabled="loading"
        @click="loadDashboard"
      />
    </PageHeading>
    <div class="stat-cards" :aria-busy="loading">
      <GlassContainer
        v-for="card in statCards"
        :key="card.key"
        :to="{ path: '/tasks', query: card.phase ? { phase: card.phase } : {} }"
        class="stat-card"
      >
        <span class="stat-label">{{ card.label }}</span
        ><span class="stat-value">{{
          loading ? "…" : loaded ? stats[card.key].toLocaleString() : "—"
        }}</span>
      </GlassContainer>
    </div>
    <GlassContainer v-if="clusterError" class="no-cluster">
      <h2>未连接 Kubernetes 集群</h2>
      <p>{{ clusterError }}</p>
    </GlassContainer>
    <GlassContainer v-else-if="error" class="error-msg" role="alert">{{ error }}</GlassContainer>
    <DashboardCharts v-if="!clusterError && !error" :stats="stats" :trend="trendData" :loading="loading" :loaded="loaded" />
  </div>
</template>
<script setup>
import { ref, onMounted, onUnmounted } from "vue";
import { api } from "@/api";
import { useClusterStatus } from "@/composables/cluster";
import GlassContainer from "@/components/GlassContainer.vue";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";
import DashboardCharts from "@/components/DashboardCharts.vue";
const { clusterError, handleApiError } = useClusterStatus();
const loading = ref(true), loaded = ref(false), error = ref("");
const stats = ref({ total: 0, running: 0, succeeded: 0, failed: 0, pending: 0, suspended: 0, onetime: 0, cron: 0, delay: 0 });
const trendData = ref([]);
const statCards = [
  { key: "total", label: "全部任务" },
  {
    key: "running",
    label: "运行中",
    phase: "Running",
  },
  {
    key: "succeeded",
    label: "已成功",
    phase: "Succeeded",
  },
  {
    key: "failed",
    label: "已失败",
    phase: "Failed",
  },
  {
    key: "suspended",
    label: "已暂停",
    phase: "Suspended",
  },
  {
    key: "pending",
    label: "等待中",
    phase: "Pending",
  },
];
let disposed = false;
onMounted(loadDashboard);
onUnmounted(() => { disposed = true; });
async function loadDashboard() {
  loading.value = true;
  error.value = "";
  clusterError.value = "";
  try {
    const [s, t] = await Promise.all([api.getStats(), api.getTrend()]);
    if (disposed) return;
    stats.value = { ...stats.value, ...s };
    trendData.value = t;
    loaded.value = true;
  } catch (e) {
    if (disposed) return;
    const message = handleApiError(e);
    if (message !== null) error.value = "加载仪表盘数据失败：" + message;
    loaded.value = false;
  } finally {
    if (!disposed) loading.value = false;
  }
}
</script>
