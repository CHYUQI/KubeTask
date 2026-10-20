<template>
  <div class="dashboard">
    <PageHeading title="总览">
      <GlassButton
        :label="loading ? '刷新中…' : '刷新'"
        :disabled="loading"
        @click="loadDashboard"
      />
      <GlassButton label="创建任务" tone="accent" to="/tasks/create" />
    </PageHeading>
    <div class="stat-cards" :aria-busy="loading">
      <router-link
        v-for="card in statCards"
        :key="card.key"
        :to="{ path: '/tasks', query: card.phase ? { phase: card.phase } : {} }"
        class="stat-card glass-panel"
      >
        <span class="stat-label">{{ card.label }}</span
        ><span class="stat-value">{{
          loading ? "…" : loaded ? stats[card.key].toLocaleString() : "—"
        }}</span>
      </router-link>
    </div>
    <NoCluster v-if="clusterError" :message="clusterError" />
    <div v-else-if="error" class="error-msg" role="alert">{{ error }}</div>
    <div v-if="!clusterError && !error" class="dashboard-charts">
      <section class="chart-panel glass-panel">
        <div class="panel-heading">
          <h2>执行趋势</h2>
          <span>全部记录</span>
        </div>
        <div v-if="loading" class="chart-placeholder">加载中…</div>
        <div
          v-else-if="!loaded || !trendData.length || error || clusterError"
          class="chart-placeholder"
        >
          {{ loaded && !error && !clusterError ? "暂无执行记录" : "暂无数据" }}
        </div>
        <div
          v-show="
            !loading && loaded && trendData.length && !error && !clusterError
          "
          ref="trendRef"
          class="trend-chart"
          aria-label="成功与失败次数的历史执行趋势图"
          role="img"
        />
      </section>
      <section class="chart-panel type-panel glass-panel">
        <div class="panel-heading"><h2>任务类型</h2></div>
        <div class="type-chart-wrap">
          <div
            ref="typeRef"
            class="type-chart"
            role="img"
            aria-label="任务类型分布图"
          />
          <div class="donut-center">
            <strong>{{
              loaded && !error && !clusterError ? stats.total : "—"
            }}</strong
            ><span>全部任务</span>
          </div>
        </div>
        <div class="type-legend">
          <div v-for="item in typeItems" :key="item.key">
            <span
              class="legend-dot"
              :style="{ background: item.color }"
            /><span>{{ item.label }}</span
            ><strong>{{
              loaded && !error && !clusterError ? stats[item.key] : "—"
            }}</strong>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
<script setup>
import { ref, onMounted, onUnmounted, nextTick } from "vue";
import * as echarts from "echarts/core";
import { LineChart, PieChart } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import { api } from "@/api";
import { useClusterStatus } from "@/composables/cluster";
import NoCluster from "@/components/NoCluster.vue";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";
echarts.use([
  LineChart,
  PieChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  CanvasRenderer,
]);
const { clusterError, handleApiError } = useClusterStatus();
const loading = ref(true),
  loaded = ref(false),
  error = ref("");
const stats = ref({
  total: 0,
  running: 0,
  succeeded: 0,
  failed: 0,
  pending: 0,
  suspended: 0,
  onetime: 0,
  cron: 0,
  delay: 0,
});
const trendData = ref([]),
  trendRef = ref(null),
  typeRef = ref(null);
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
const typeItems = [
  { key: "onetime", label: "OneTime", color: "#7790ac" },
  { key: "cron", label: "Cron", color: "#9bb7b0" },
  { key: "delay", label: "Delay", color: "#bac2ce" },
];
let trendChart,
  typeChart,
  observer,
  disposed = false;
onMounted(() => {
  observer = new ResizeObserver(() => {
    trendChart?.resize();
    typeChart?.resize();
  });
  if (trendRef.value) observer.observe(trendRef.value);
  if (typeRef.value) observer.observe(typeRef.value);
  loadDashboard();
});
onUnmounted(() => {
  disposed = true;
  observer?.disconnect();
  trendChart?.dispose();
  typeChart?.dispose();
});
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
    if (!disposed) {
      loading.value = false;
      await nextTick();
      if (!disposed) renderCharts();
    }
  }
}
function renderCharts() {
  // Error states remove the chart DOM. Recreate instances when a successful
  // refresh mounts new containers instead of painting into detached nodes.
  if (trendChart && trendChart.getDom() !== trendRef.value) {
    trendChart.dispose();
    trendChart = undefined;
  }
  if (typeChart && typeChart.getDom() !== typeRef.value) {
    typeChart.dispose();
    typeChart = undefined;
  }
  observer?.disconnect();
  if (trendRef.value) observer?.observe(trendRef.value);
  if (typeRef.value) observer?.observe(typeRef.value);
  const rootStyle = getComputedStyle(document.documentElement);
  const captionSize = (parseFloat(rootStyle.fontSize) * 13) / 16;
  const motion = !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (trendRef.value && loaded.value && trendData.value.length) {
    trendChart ||= echarts.init(trendRef.value);
    const sorted = [...trendData.value].sort((a, b) =>
      a.date.localeCompare(b.date),
    );
    trendChart.setOption({
      textStyle: { fontFamily: rootStyle.fontFamily, fontSize: captionSize },
      animation: motion,
      animationDuration: 180,
      animationDurationUpdate: 180,
      tooltip: {
        trigger: "axis",
        backgroundColor: "rgba(255,255,255,.94)",
        borderColor: "#e3e6f0",
        textStyle: { color: "#1d1d1f", fontSize: captionSize },
      },
      legend: {
        data: ["成功", "失败"],
        bottom: 0,
        itemWidth: 18,
        itemHeight: 7,
        textStyle: { color: "#63636a", fontSize: captionSize },
      },
      grid: { left: 38, right: 16, top: 22, bottom: 55 },
      xAxis: {
        type: "category",
        boundaryGap: false,
        data: sorted.map((d) => d.date.slice(5)),
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: { color: "#63636a", fontSize: captionSize, margin: 14 },
      },
      yAxis: {
        type: "value",
        minInterval: 1,
        splitLine: {
          lineStyle: { color: "rgba(130,145,175,.13)", type: "dashed" },
        },
        axisLabel: { color: "#63636a", fontSize: captionSize },
      },
      series: [
        {
          name: "成功",
          type: "line",
          data: sorted.map((d) => d.succeeded),
          smooth: 0.4,
          symbol: "circle",
          showSymbol: false,
          symbolSize: 7,
          itemStyle: { color: "#617b9b" },
          lineStyle: { width: 3 },
          areaStyle: { color: "rgba(98,119,145,.07)" },
        },
        {
          name: "失败",
          type: "line",
          data: sorted.map((d) => d.failed),
          smooth: 0.4,
          symbol: "circle",
          showSymbol: false,
          itemStyle: { color: "#bd8790" },
          lineStyle: { width: 2, type: "dashed" },
        },
      ],
    });
    trendChart.resize();
  }
  if (typeRef.value) {
    typeChart ||= echarts.init(typeRef.value);
    const hasData =
      loaded.value &&
      !error.value &&
      !clusterError.value &&
      typeItems.some((item) => stats.value[item.key] > 0);
    typeChart.setOption({
      textStyle: { fontFamily: rootStyle.fontFamily, fontSize: captionSize },
      animation: motion,
      animationDuration: 180,
      animationDurationUpdate: 180,
      tooltip: { show: hasData, trigger: "item" },
      series: [
        {
          type: "pie",
          radius: ["73%", "90%"],
          center: ["50%", "50%"],
          label: { show: false },
          emphasis: { scaleSize: 3 },
          itemStyle: {
            borderRadius: 5,
            borderColor: "rgba(255,255,255,.8)",
            borderWidth: 4,
          },
          data: hasData
            ? typeItems.map((item) => ({
                value: stats.value[item.key],
                name: item.label,
                itemStyle: { color: item.color },
              }))
            : [
                {
                  value: 1,
                  itemStyle: { color: "#e5e6f0" },
                  tooltip: { show: false },
                },
              ],
        },
      ],
    });
    typeChart.resize();
  }
}
</script>
