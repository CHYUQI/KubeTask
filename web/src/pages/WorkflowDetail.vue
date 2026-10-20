<template>
  <div class="workflow-detail">
    <PageHeading
      :title="String(route.params.name)"
      back-to="/workflows"
      back-label="工作流"
    >
      <div class="refresh-controls">
        <label
          ><input v-model="autoRefresh" type="checkbox" /> 每 5
          秒刷新运行状态</label
        >
        <GlassButton
          :label="refreshing ? '刷新中…' : '刷新'"
          :disabled="loading || refreshing"
          @click="fetchWorkflow()"
        />
      </div>
    </PageHeading>

    <div v-if="loading" class="loading">加载中...</div>
    <NoCluster v-else-if="clusterError" :message="clusterError" />
    <div v-else-if="error" class="error-msg" role="alert">{{ error }}</div>

    <template v-else-if="workflow">
      <div v-if="refreshError" class="warning" role="alert">
        自动刷新失败，当前显示上次成功获取的数据：{{ refreshError }}
      </div>
      <section
        class="card glass-panel workflow-summary"
        aria-labelledby="workflow-info"
      >
        <h2 id="workflow-info">基本信息</h2>
        <dl class="summary-grid">
          <div>
            <dt>状态</dt>
            <dd>
              <span class="tag" :class="phaseClass(workflow.status?.phase)">{{
                workflow.status?.phase || "未初始化"
              }}</span>
              <span v-if="workflow.metadata.deletionTimestamp" class="deleting"
                >删除中，正在清理子 Task</span
              >
            </dd>
          </div>
          <div>
            <dt>节点数</dt>
            <dd>{{ nodes.length }}</dd>
          </div>
          <div>
            <dt>并行上限</dt>
            <dd>{{ workflow.spec?.maxParallel || "不限" }}</dd>
          </div>
          <div>
            <dt>创建时间</dt>
            <dd>{{ formatTime(workflow.metadata.creationTimestamp) }}</dd>
          </div>
          <div>
            <dt>开始时间</dt>
            <dd>{{ formatTime(workflow.status?.startTime) }}</dd>
          </div>
          <div>
            <dt>完成时间</dt>
            <dd>{{ formatTime(workflow.status?.completionTime) }}</dd>
          </div>
          <div v-if="workflow.status?.message" class="summary-message">
            <dt>状态说明</dt>
            <dd class="message">
              {{ workflow.status.message }}
            </dd>
          </div>
        </dl>
      </section>

      <div v-if="graph?.validationError" class="warning" role="alert">
        工作流定义校验未通过：{{ graph.validationError }}
      </div>

      <WorkflowGraph :graph="graph" />

      <section class="card glass-panel" aria-labelledby="workflow-nodes">
        <h2 id="workflow-nodes">节点状态</h2>
        <div v-if="nodes.length" class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>节点</th>
                <th>状态</th>
                <th>依赖节点</th>
                <th>失败分支</th>
                <th>子 Task</th>
                <th>开始时间</th>
                <th>完成时间</th>
                <th>说明</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="node in nodes" :key="node.name">
                <td>{{ node.name }}</td>
                <td>
                  <span class="tag" :class="phaseClass(node.phase)">{{
                    node.phase || "Pending"
                  }}</span>
                </td>
                <td>{{ node.dependsOn?.join(", ") || "-" }}</td>
                <td>{{ node.runOnFailure ? "是" : "否" }}</td>
                <td>
                  <template v-if="node.taskName">
                    <router-link
                      :to="{
                        name: 'TaskDetail',
                        params: { name: node.taskName },
                      }"
                      >{{ node.taskName }}</router-link
                    >
                    <router-link
                      class="log-link"
                      :to="{
                        name: 'TaskLogs',
                        params: { name: node.taskName },
                      }"
                      :aria-label="node.name + ' 的日志'"
                      >日志</router-link
                    >
                  </template>
                  <span v-else>-</span>
                </td>
                <td class="time-cell">{{ formatTime(node.startTime) }}</td>
                <td class="time-cell">{{ formatTime(node.completionTime) }}</td>
                <td class="message-cell">{{ node.message || "-" }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty">暂无节点</div>
      </section>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from "vue";
import { useRoute } from "vue-router";
import { api } from "@/api";
import { useClusterStatus } from "@/composables/cluster";
import NoCluster from "@/components/NoCluster.vue";
import WorkflowGraph from "@/components/WorkflowGraph.vue";
import PageHeading from "@/components/PageHeading.vue";
import GlassButton from "@/components/GlassButton.vue";

const route = useRoute();
const { clusterError, handleApiError } = useClusterStatus();
const loading = ref(true);
const error = ref("");
const refreshing = ref(false);
const refreshError = ref("");
const autoRefresh = ref(true);
const workflow = ref(null);
const graph = ref(null);
const nodes = computed(() => graph.value?.nodes || []);
let requestId = 0;
let refreshTimer;

watch(
  () => route.params.name,
  () => fetchWorkflow(),
  { immediate: true },
);
onMounted(() => {
  refreshTimer = window.setInterval(() => {
    if (
      !autoRefresh.value ||
      document.hidden ||
      loading.value ||
      refreshing.value ||
      !workflow.value
    )
      return;
    const terminal = ["Succeeded", "Failed"].includes(
      workflow.value.status?.phase,
    );
    if (!terminal || workflow.value.metadata.deletionTimestamp)
      fetchWorkflow(true);
  }, 5000);
});
onBeforeUnmount(() => {
  requestId++;
  window.clearInterval(refreshTimer);
});

async function fetchWorkflow(background = false) {
  const currentRequest = ++requestId;
  if (background) {
    refreshing.value = true;
  } else {
    loading.value = true;
    refreshing.value = false;
    error.value = "";
    clusterError.value = "";
    workflow.value = null;
    graph.value = null;
  }
  try {
    const data = await api.getWorkflow(route.params.name);
    // 切换工作流后，忽略上一个详情请求的迟到响应。
    if (currentRequest !== requestId) return;
    workflow.value = data.workflow;
    graph.value = data.graph;
    refreshError.value = "";
  } catch (e) {
    if (currentRequest !== requestId) return;
    if (e.message === "workflow not found") {
      error.value = "工作流不存在或已被删除";
      workflow.value = null;
      graph.value = null;
    } else if (background) {
      refreshError.value = e.message;
    } else {
      const message = handleApiError(e);
      if (message !== null) error.value = "加载工作流详情失败: " + message;
    }
  } finally {
    if (currentRequest === requestId) {
      loading.value = false;
      refreshing.value = false;
    }
  }
}

function phaseClass(phase) {
  return (
    {
      Pending: "pending",
      Running: "running",
      Succeeded: "succeeded",
      Failed: "failed",
      Skipped: "skipped",
    }[phase] || "pending"
  );
}

function formatTime(value) {
  if (!value) return "-";
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}
</script>
