<template>
  <section class="workflow-graph glass-panel" aria-labelledby="dag-heading">
    <div class="graph-header">
      <h2 id="dag-heading">依赖图</h2>
      <div class="zoom-controls" aria-label="图形缩放">
        <button
          :disabled="zoom <= MIN_ZOOM"
          aria-label="缩小依赖图"
          @click="changeZoom(-0.1)"
        >
          −
        </button>
        <span>{{ Math.round(zoom * 100) }}%</span>
        <button
          :disabled="zoom >= MAX_ZOOM"
          aria-label="放大依赖图"
          @click="changeZoom(0.1)"
        >
          +
        </button>
        <button @click="fitGraph">适应窗口</button>
      </div>
    </div>

    <div class="legend" aria-label="节点状态图例">
      <span v-for="phase in phases" :key="phase" :class="phase.toLowerCase()"
        ><i></i>{{ phase }}</span
      >
      <span>虚线：失败分支</span>
    </div>
    <p class="hint">依赖方向从左到右。点击节点查看详情，图形可滚动和缩放。</p>

    <div v-if="!layout.nodes.length" class="empty">暂无节点</div>
    <div v-else ref="viewport" class="graph-viewport" aria-label="工作流依赖图">
      <div
        class="scaled-canvas"
        :style="{
          width: layout.width * zoom + 'px',
          height: layout.height * zoom + 'px',
        }"
      >
        <div
          class="graph-canvas"
          :style="{
            width: layout.width + 'px',
            height: layout.height + 'px',
            transform: `scale(${zoom})`,
          }"
        >
          <svg :width="layout.width" :height="layout.height" aria-hidden="true">
            <defs>
              <marker
                :id="arrowId"
                viewBox="0 0 10 10"
                refX="9"
                refY="5"
                markerWidth="7"
                markerHeight="7"
                orient="auto-start-reverse"
              >
                <path d="M 0 0 L 10 5 L 0 10 z" fill="#94a3b8" />
              </marker>
            </defs>
            <polyline
              v-for="edge in layout.edges"
              :key="edge.id"
              :points="edge.points"
              :class="{ 'failure-edge': edge.failureBranch }"
              :marker-end="`url(#${arrowId})`"
            />
          </svg>
          <button
            v-for="node in layout.nodes"
            :key="node.name"
            class="graph-node"
            :class="[
              node.missing
                ? 'missing'
                : (node.phase || 'Pending').toLowerCase(),
              { selected: selectedName === node.name },
            ]"
            :style="{
              left: node.x + 'px',
              top: node.y + 'px',
              width: NODE_WIDTH + 'px',
              height: NODE_HEIGHT + 'px',
            }"
            :aria-label="
              node.name +
              '：' +
              (node.missing ? '缺失节点' : node.phase || 'Pending')
            "
            :aria-pressed="selectedName === node.name"
            :title="node.name"
            @click="selectedName = node.name"
          >
            <strong>{{ node.name }}</strong>
            <span>{{
              node.missing ? "缺失节点" : node.phase || "Pending"
            }}</span>
            <small v-if="node.runOnFailure">失败分支</small>
          </button>
        </div>
      </div>
    </div>

    <div v-if="selectedNode" class="node-detail" aria-live="polite">
      <h3>{{ selectedNode.name }}</h3>
      <p>
        状态：{{
          selectedNode.missing ? "缺失节点" : selectedNode.phase || "Pending"
        }}
      </p>
      <p>依赖：{{ selectedNode.dependsOn?.join(", ") || "-" }}</p>
      <p v-if="selectedNode.runOnFailure">
        失败分支：全部依赖到达终态，且至少一个为 Failed / Skipped 时执行。
      </p>
      <p v-if="selectedNode.message" class="node-message">
        {{ selectedNode.message }}
      </p>
      <div v-if="selectedNode.taskName" class="node-links">
        <router-link
          :to="{ name: 'TaskDetail', params: { name: selectedNode.taskName } }"
          >查看子 Task：{{ selectedNode.taskName }}</router-link
        >
        <router-link
          :to="{ name: 'TaskLogs', params: { name: selectedNode.taskName } }"
          >查看节点日志</router-link
        >
      </div>
    </div>
  </section>
</template>

<script setup>
import { ref, computed, watch, nextTick, onMounted, useId } from "vue";
import { layoutWorkflow, NODE_WIDTH, NODE_HEIGHT } from "@/workflow/layout";

const props = defineProps({ graph: { type: Object, default: () => ({}) } });
const phases = ["Pending", "Running", "Succeeded", "Failed", "Skipped"];
const MIN_ZOOM = 0.25;
const MAX_ZOOM = 2;
const viewport = ref(null);
const zoom = ref(1);
const selectedName = ref("");
const arrowId = "dag-arrow-" + useId();
const layout = computed(() => layoutWorkflow(props.graph || {}));
const selectedNode = computed(() =>
  layout.value.nodes.find((node) => node.name === selectedName.value),
);

function changeZoom(delta) {
  zoom.value = Math.min(
    MAX_ZOOM,
    Math.max(MIN_ZOOM, Math.round((zoom.value + delta) * 100) / 100),
  );
}

function fitGraph() {
  if (!viewport.value || !layout.value.width) return;
  zoom.value = Math.max(
    MIN_ZOOM,
    Math.min(
      1,
      (viewport.value.clientWidth - 16) / layout.value.width,
      (viewport.value.clientHeight - 16) / layout.value.height,
    ),
  );
  viewport.value.scrollTo(0, 0);
}

onMounted(fitGraph);
// Status refreshes preserve the selected node and zoom; fit only if dimensions change.
watch(
  () => `${layout.value.width}:${layout.value.height}`,
  async () => {
    await nextTick();
    fitGraph();
  },
);
</script>

<style scoped>
.workflow-graph {
  padding: 24px;
  margin-bottom: 24px;
}
.graph-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
}
h2 {
  font-size: var(--text-section);
  line-height: 1.3;
  font-weight: 600;
  color: #1d1d1f;
}
.zoom-controls {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--text-caption);
}
.zoom-controls button {
  min-height: var(--control-height);
  padding: 5px 12px;
  border: 1px solid #d2d2d7;
  border-radius: 8px;
  background: #fff;
  cursor: pointer;
}
.zoom-controls button:disabled {
  opacity: 0.4;
  cursor: default;
}
.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: var(--text-caption);
  color: #6e6e73;
}
.legend span {
  display: flex;
  align-items: center;
  gap: 5px;
}
.legend i {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}
.hint {
  font-size: var(--text-caption);
  color: #6e6e73;
  margin: 12px 0 20px;
}
.graph-viewport {
  overflow: auto;
  height: clamp(280px, 48vh, 420px);
  border: 1px solid #e5e5ea;
  border-radius: 10px;
  background-color: #f5f5f7;
}
.scaled-canvas {
  position: relative;
}
.graph-canvas {
  position: absolute;
  transform-origin: top left;
}
svg {
  position: absolute;
  inset: 0;
  pointer-events: none;
}
polyline {
  fill: none;
  stroke: #94a3b8;
  stroke-width: 2;
}
.failure-edge {
  stroke-dasharray: 6 4;
}
.graph-node {
  position: absolute;
  padding: 12px;
  border: 1.5px solid currentColor;
  border-radius: 12px;
  background: #fff;
  text-align: left;
  cursor: pointer;
  font-family: inherit;
}
.graph-node strong {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--text-body);
}
.graph-node span {
  display: inline-block;
  margin-top: 8px;
  font-size: var(--text-caption);
}
.graph-node small {
  float: right;
  margin-top: 8px;
  font-size: var(--text-small);
}
.graph-node.selected,
.graph-node:focus-visible {
  outline: 3px solid #007aff80;
  outline-offset: 3px;
}
.pending {
  color: #6e6e73;
}
.running {
  color: #007aff;
}
.succeeded {
  color: #248a3d;
}
.failed {
  color: #d03535;
}
.skipped {
  color: #a96800;
}
.missing {
  color: #d03535;
  border-style: dashed;
}
.node-detail {
  margin-top: 20px;
  padding: 16px;
  border-radius: 10px;
  background: #f5f5f7;
  font-size: var(--text-control);
  overflow-wrap: anywhere;
}
h3 {
  font-size: var(--text-body);
  margin-bottom: 8px;
}
.node-detail p {
  margin-top: 6px;
}
.node-message {
  white-space: pre-wrap;
}
.node-links {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin-top: 12px;
}
.node-links a {
  color: #007aff;
}
.node-links a:hover {
  text-decoration: underline;
}
.empty {
  text-align: center;
  padding: 40px;
  color: #6e6e73;
}
@media (max-width: 600px) {
  .workflow-graph {
    padding: 20px;
  }
}
</style>
