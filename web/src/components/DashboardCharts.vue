<template>
  <div class="dashboard-charts">
    <GlassContainer as="section" class="chart-panel">
      <div class="panel-heading"><h2>执行趋势</h2><span>全部记录</span></div>
      <div v-if="loading || !loaded || !trend.length" class="chart-placeholder">
        {{ loading ? '加载中…' : loaded ? '暂无执行记录' : '暂无数据' }}
      </div>
      <div v-else ref="trendHost" class="trend-chart">
        <svg :viewBox="`0 0 ${plot.width} 230`" class="trend-svg"
          role="group" aria-label="成功与失败次数的历史执行趋势图"
          @pointermove="inspectPoint" @pointerleave="active = -1">
          <g v-for="tick in plot.ticks" :key="tick.value" class="chart-axis">
            <line :x1="plot.left" :x2="plot.right" :y1="tick.y" :y2="tick.y" />
            <text :x="plot.left - 8" :y="tick.y + 4" text-anchor="end">{{ tick.value }}</text>
          </g>
          <template v-for="(point, index) in plot.points" :key="point.date">
            <text v-if="index % plot.labelEvery === 0 || index === plot.points.length - 1"
              class="chart-date" :x="point.x" y="216"
              :text-anchor="index === 0 ? 'start' : index === plot.points.length - 1 ? 'end' : 'middle'">
              {{ point.date.slice(5) }}
            </text>
            <circle :cx="point.x" :cy="point.successY" r="10" fill="transparent" tabindex="0"
              role="img" :aria-label="`${point.date}，成功 ${point.succeeded}，失败 ${point.failed}`"
              @focus="active = index" @blur="active = -1" @keydown="pointKey($event, index)">
              <title>{{ point.date }}：成功 {{ point.succeeded }}，失败 {{ point.failed }}</title>
            </circle>
          </template>
          <path v-if="showSuccess" :d="plot.successPath" class="trend-success" />
          <path v-if="showFailure" :d="plot.failurePath" class="trend-failure" />
          <g v-for="(point, index) in plot.points" :key="'dot-' + point.date">
            <circle v-if="showSuccess && (plot.points.length === 1 || index === active)"
              :cx="point.x" :cy="point.successY" r="3.5" class="trend-success-point" />
            <circle v-if="showFailure && (plot.points.length === 1 || index === active)"
              :cx="point.x" :cy="point.failureY" r="3" class="trend-failure-point" />
          </g>
          <line v-if="activePoint" :x1="activePoint.x" :x2="activePoint.x" y1="14" y2="190" class="chart-cursor" />
        </svg>
        <GlassContainer v-if="activePoint" class="chart-tooltip" role="status"
          :style="{ left: `${Math.min(100 - 60, Math.max(0, activePoint.x / plot.width * 100 - 10))}%` }">
          <strong>{{ activePoint.date }}</strong>
          <span>成功 {{ activePoint.succeeded }} · 失败 {{ activePoint.failed }}</span>
        </GlassContainer>
        <div class="trend-legend" aria-label="趋势图系列">
          <button type="button" :aria-pressed="showSuccess" @click="showSuccess = !showSuccess">
            <i class="success-key" />成功
          </button>
          <button type="button" :aria-pressed="showFailure" @click="showFailure = !showFailure">
            <i class="failure-key" />失败
          </button>
        </div>
      </div>
    </GlassContainer>
    <GlassContainer as="section" class="chart-panel type-panel">
      <div class="panel-heading"><h2>任务类型</h2></div>
      <div class="type-chart-wrap">
        <svg viewBox="0 0 156 156" class="type-chart" role="group" aria-label="任务类型分布图">
          <circle cx="78" cy="78" r="66" class="donut-track" />
          <circle v-for="segment in segments" :key="segment.key" cx="78" cy="78" r="66"
            fill="none" :stroke-width="segment.width" stroke-linecap="round" :stroke="segment.color" pathLength="100"
            :stroke-dasharray="`${segment.length} ${100 - segment.length}`"
            :stroke-dashoffset="-segment.offset" transform="rotate(-90 78 78)"
            tabindex="0" role="img" :aria-label="`${segment.label}：${segment.value} 个，${Math.round(segment.percent)}%`">
            <title>{{ segment.label }}：{{ segment.value }} 个（{{ Math.round(segment.percent) }}%）</title>
          </circle>
        </svg>
        <div class="donut-center"><strong>{{ loaded ? stats.total : '—' }}</strong><span>全部任务</span></div>
      </div>
      <div class="type-legend">
        <div v-for="item in typeItems" :key="item.key">
          <span class="legend-dot" :style="{ background: item.color }" />
          <span>{{ item.label }}</span><strong>{{ loaded ? stats[item.key] : '—' }}</strong>
        </div>
      </div>
    </GlassContainer>
  </div>
</template>
<script>
const count = value => Number.isFinite(Number(value)) ? Math.max(0, Number(value)) : 0;
export const typeItems = [
  { key: 'onetime', label: 'OneTime', color: '#7790ac' },
  { key: 'cron', label: 'Cron', color: '#9bb7b0' },
  { key: 'delay', label: 'Delay', color: '#bac2ce' },
];
export function trendGeometry(data, width = 600) {
  width = Math.max(180, width);
  const rows = [...data].sort((a, b) => a.date.localeCompare(b.date));
  const maximum = Math.max(1, ...rows.flatMap(row => [count(row.succeeded), count(row.failed)]));
  const magnitude = 10 ** Math.floor(Math.log10(maximum / 4));
  const step = Math.max(1, [1, 2, 5, 10].find(value => value * magnitude >= maximum / 4) * magnitude);
  const ceiling = Math.ceil(maximum / step) * step;
  const left = 38, right = width - 12, y = value => 190 - count(value) / ceiling * 176;
  const points = rows.map((row, index) => ({
    ...row, succeeded: count(row.succeeded), failed: count(row.failed),
    x: rows.length === 1 ? (left + right) / 2 : left + index / (rows.length - 1) * (right - left),
    successY: y(row.succeeded), failureY: y(row.failed),
  }));
  const line = key => points.map((point, index) => `${index ? 'L' : 'M'}${point.x},${point[key]}`).join(' ');
  return { width, left, right, points, successPath: line('successY'), failurePath: line('failureY'),
    ticks: Array.from({ length: Math.round(ceiling / step) + 1 }, (_, index) => ({ value: index * step, y: y(index * step) })),
    labelEvery: Math.max(1, Math.ceil(rows.length / Math.max(2, (right - left) / 72))),
  };
}
export function typeSegments(stats) {
  const items = typeItems.map(item => ({ ...item, value: count(stats[item.key]) })).filter(item => item.value > 0);
  const total = items.reduce((sum, item) => sum + item.value, 0);
  let offset = 0;
  return items.map(item => {
    const percent = item.value / total * 100;
    const arcLength = percent / 100 * 2 * Math.PI * 66;
    const width = Math.min(14, arcLength * 0.6);
    const gap = Math.min(3, arcLength * 0.2) / (2 * Math.PI * 66) * 100;
    const cap = width / (2 * Math.PI * 66) * 100;
    const single = items.length === 1;
    const segment = { ...item, percent, width: single ? 14 : width,
      offset: single ? 0 : offset + (gap + cap) / 2,
      length: single ? 100 : percent - gap - cap };
    offset += percent;
    return segment;
  });
}
</script>
<script setup>
import { computed, onUnmounted, ref, watch } from 'vue';
import GlassContainer from './GlassContainer.vue';
const props = defineProps({ stats: { type: Object, required: true }, trend: { type: Array, default: () => [] }, loading: Boolean, loaded: Boolean });
const trendHost = ref(null), width = ref(600), active = ref(-1), showSuccess = ref(true), showFailure = ref(true);
const plot = computed(() => trendGeometry(props.trend, width.value));
const segments = computed(() => props.loaded ? typeSegments(props.stats) : []);
const activePoint = computed(() => plot.value.points[active.value]);
let observer;
watch(trendHost, host => {
  observer?.disconnect();
  if (!host) return;
  width.value = host.clientWidth;
  observer = new ResizeObserver(([entry]) => { width.value = entry.contentRect.width; });
  observer.observe(host);
}, { flush: 'post' });
watch(() => props.trend, () => { active.value = -1; });
onUnmounted(() => observer?.disconnect());
function inspectPoint(event) {
  const rect = event.currentTarget.getBoundingClientRect();
  const x = (event.clientX - rect.left) / rect.width * plot.value.width;
  active.value = Math.max(0, Math.min(plot.value.points.length - 1,
    Math.round((x - plot.value.left) / (plot.value.right - plot.value.left) * (plot.value.points.length - 1))));
}
function pointKey(event, index) {
  const points = [...event.currentTarget.ownerSVGElement.querySelectorAll('[tabindex="0"]')];
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? points.length - 1 :
    event.key === 'ArrowRight' ? Math.min(points.length - 1, index + 1) : event.key === 'ArrowLeft' ? Math.max(0, index - 1) : -1;
  if (next >= 0) { event.preventDefault(); points[next].focus(); }
  else if (event.key === 'Escape') active.value = -1;
}
</script>
