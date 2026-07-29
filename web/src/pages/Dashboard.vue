<template>
  <div class="dashboard">
    <h1>仪表盘</h1>

    <div v-if="loading" class="loading">加载中...</div>

    <NoCluster v-else-if="clusterError" :message="clusterError" />

    <div v-else-if="error" class="error">{{ error }}</div>

    <div v-else class="stat-cards">
      <div class="card total"><span class="num">{{ stats.total }}</span><span class="label">总任务</span></div>
      <div class="card running"><span class="num">{{ stats.running }}</span><span class="label">运行中</span></div>
      <div class="card succeeded"><span class="num">{{ stats.succeeded }}</span><span class="label">已成功</span></div>
      <div class="card failed"><span class="num">{{ stats.failed }}</span><span class="label">已失败</span></div>
      <div class="card suspended"><span class="num">{{ stats.suspended }}</span><span class="label">已暂停</span></div>
      <div class="card pending"><span class="num">{{ stats.pending }}</span><span class="label">等待中</span></div>
    </div>

    <div class="charts">
      <div class="chart-box">
        <h3>执行趋势</h3>
        <div ref="trendRef" class="chart"></div>
      </div>
      <div class="chart-box">
        <h3>类型分布</h3>
        <div ref="typeRef" class="chart"></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import * as echarts from 'echarts'
import { api } from '@/api'
import { useClusterStatus } from '@/composables/cluster'
import NoCluster from '@/components/NoCluster.vue'

const { clusterError, handleApiError } = useClusterStatus()

const loading = ref(true)
const error = ref('')
const stats = ref({ total: 0, running: 0, succeeded: 0, failed: 0, pending: 0, suspended: 0, onetime: 0, cron: 0, delay: 0 })
const trendData = ref([])
const trendRef = ref(null)
const typeRef = ref(null)

let trendChart = null
let typeChart = null

onMounted(async () => {
  try {
    const [s, t] = await Promise.all([api.getStats(), api.getTrend()])
    stats.value = s
    trendData.value = t
    loading.value = false
    await nextTick()
    initTrendChart()
    initTypeChart()
  } catch (e) {
    const msg = handleApiError(e)
    if (msg === null) {
      // cluster error already set by handleApiError
    } else {
      error.value = '加载仪表盘数据失败: ' + msg
    }
    loading.value = false
  }
})

onUnmounted(() => {
  trendChart?.dispose()
  typeChart?.dispose()
})

function initTrendChart() {
  if (!trendRef.value || trendData.value.length === 0) return
  if (trendChart) trendChart.dispose()
  trendChart = echarts.init(trendRef.value)
  const sorted = [...trendData.value].sort((a, b) => a.date.localeCompare(b.date))
  trendChart.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['成功', '失败'], top: 0 },
    grid: { left: 40, right: 20, top: 40, bottom: 30 },
    xAxis: { type: 'category', data: sorted.map(d => d.date.slice(5)) },
    yAxis: { type: 'value', minInterval: 1 },
    series: [
      { name: '成功', type: 'line', data: sorted.map(d => d.succeeded), smooth: true, color: '#22c55e' },
      { name: '失败', type: 'line', data: sorted.map(d => d.failed), smooth: true, color: '#ef4444' }
    ]
  })
}

function initTypeChart() {
  if (!typeRef.value) return
  if (typeChart) typeChart.dispose()
  typeChart = echarts.init(typeRef.value)
  typeChart.setOption({
    tooltip: { trigger: 'item' },
    legend: { orient: 'vertical', left: 0, top: 20 },
    series: [{
      name: '任务类型',
      type: 'pie',
      radius: ['50%', '75%'],
      center: ['60%', '50%'],
      label: { show: false },
      data: [
        { value: stats.value.cron, name: 'Cron', itemStyle: { color: '#3b82f6' } },
        { value: stats.value.onetime, name: 'OneTime', itemStyle: { color: '#8b5cf6' } },
        { value: stats.value.delay, name: 'Delay', itemStyle: { color: '#f59e0b' } }
      ]
    }]
  })
}
</script>

<style scoped>
.dashboard h1 { font-size: 22px; margin-bottom: 20px; }
.loading, .error { padding: 40px; text-align: center; }
.error { color: #ef4444; }
.stat-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 14px; margin-bottom: 24px; }
.card { background: #fff; border-radius: 8px; padding: 18px; border-left: 4px solid #e2e8f0; }
.card .num { display: block; font-size: 28px; font-weight: bold; }
.card .label { color: #64748b; font-size: 13px; }
.card.total { border-left-color: #6366f1; } .card.total .num { color: #6366f1; }
.card.running { border-left-color: #3b82f6; } .card.running .num { color: #3b82f6; }
.card.succeeded { border-left-color: #22c55e; } .card.succeeded .num { color: #22c55e; }
.card.failed { border-left-color: #ef4444; } .card.failed .num { color: #ef4444; }
.card.suspended { border-left-color: #f59e0b; } .card.suspended .num { color: #f59e0b; }
.card.pending { border-left-color: #94a3b8; } .card.pending .num { color: #94a3b8; }
.charts { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.chart-box { background: #fff; border-radius: 8px; padding: 16px; }
.chart-box h3 { font-size: 15px; margin-bottom: 10px; }
.chart { width: 100%; height: 280px; }
</style>
