<template>
  <div class="workflow-detail">
    <div class="header">
      <router-link to="/workflows" class="back">← 返回工作流列表</router-link>
      <div class="refresh-controls">
        <label><input v-model="autoRefresh" type="checkbox" /> 每 5 秒刷新运行状态</label>
        <button :disabled="loading || refreshing" @click="fetchWorkflow()">{{ refreshing ? '刷新中...' : '刷新' }}</button>
      </div>
    </div>
    <h1>{{ route.params.name }}</h1>

    <div v-if="loading" class="loading">加载中...</div>
    <NoCluster v-else-if="clusterError" :message="clusterError" />
    <div v-else-if="error" class="error-msg" role="alert">{{ error }}</div>

    <template v-else-if="workflow">
      <div v-if="refreshError" class="warning" role="alert">自动刷新失败，当前显示上次成功获取的数据：{{ refreshError }}</div>
      <section class="card" aria-labelledby="workflow-info">
        <h2 id="workflow-info">基本信息</h2>
        <dl>
          <dt>状态</dt>
          <dd>
            <span class="tag" :class="phaseClass(workflow.status?.phase)">{{ workflow.status?.phase || '未初始化' }}</span>
            <span v-if="workflow.metadata.deletionTimestamp" class="deleting">删除中，正在清理子 Task</span>
          </dd>
          <dt>节点数</dt><dd>{{ nodes.length }}</dd>
          <dt>并行上限</dt><dd>{{ workflow.spec?.maxParallel || '不限' }}</dd>
          <dt>创建时间</dt><dd>{{ formatTime(workflow.metadata.creationTimestamp) }}</dd>
          <dt>开始时间</dt><dd>{{ formatTime(workflow.status?.startTime) }}</dd>
          <dt>完成时间</dt><dd>{{ formatTime(workflow.status?.completionTime) }}</dd>
          <dt v-if="workflow.status?.message">状态说明</dt>
          <dd v-if="workflow.status?.message" class="message">{{ workflow.status.message }}</dd>
        </dl>
      </section>

      <div v-if="graph?.validationError" class="warning" role="alert">
        工作流定义校验未通过：{{ graph.validationError }}
      </div>

      <WorkflowGraph :graph="graph" />

      <section class="card" aria-labelledby="workflow-nodes">
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
                <td><span class="tag" :class="phaseClass(node.phase)">{{ node.phase || 'Pending' }}</span></td>
                <td>{{ node.dependsOn?.join(', ') || '-' }}</td>
                <td>{{ node.runOnFailure ? '是' : '否' }}</td>
                <td>
                  <template v-if="node.taskName">
                    <router-link :to="{ name: 'TaskDetail', params: { name: node.taskName } }">{{ node.taskName }}</router-link>
                    <router-link class="log-link" :to="{ name: 'TaskLogs', params: { name: node.taskName } }" :aria-label="node.name + ' 的日志'">日志</router-link>
                  </template>
                  <span v-else>-</span>
                </td>
                <td class="time-cell">{{ formatTime(node.startTime) }}</td>
                <td class="time-cell">{{ formatTime(node.completionTime) }}</td>
                <td class="message-cell">{{ node.message || '-' }}</td>
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
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '@/api'
import { useClusterStatus } from '@/composables/cluster'
import NoCluster from '@/components/NoCluster.vue'
import WorkflowGraph from '@/components/WorkflowGraph.vue'

const route = useRoute()
const { clusterError, handleApiError } = useClusterStatus()
const loading = ref(true)
const error = ref('')
const refreshing = ref(false)
const refreshError = ref('')
const autoRefresh = ref(true)
const workflow = ref(null)
const graph = ref(null)
const nodes = computed(() => graph.value?.nodes || [])
let requestId = 0
let refreshTimer

watch(() => route.params.name, () => fetchWorkflow(), { immediate: true })
onMounted(() => {
  refreshTimer = window.setInterval(() => {
    if (!autoRefresh.value || document.hidden || loading.value || refreshing.value || !workflow.value) return
    const terminal = ['Succeeded', 'Failed'].includes(workflow.value.status?.phase)
    if (!terminal || workflow.value.metadata.deletionTimestamp) fetchWorkflow(true)
  }, 5000)
})
onBeforeUnmount(() => {
  requestId++
  window.clearInterval(refreshTimer)
})

async function fetchWorkflow(background = false) {
  const currentRequest = ++requestId
  if (background) {
    refreshing.value = true
  } else {
    loading.value = true
    refreshing.value = false
    error.value = ''
    clusterError.value = ''
    workflow.value = null
    graph.value = null
  }
  try {
    const data = await api.getWorkflow(route.params.name)
    // 切换工作流后，忽略上一个详情请求的迟到响应。
    if (currentRequest !== requestId) return
    workflow.value = data.workflow
    graph.value = data.graph
    refreshError.value = ''
  } catch (e) {
    if (currentRequest !== requestId) return
    if (e.message === 'workflow not found') {
      error.value = '工作流不存在或已被删除'
      workflow.value = null
      graph.value = null
    } else if (background) {
      refreshError.value = e.message
    } else {
      const message = handleApiError(e)
      if (message !== null) error.value = '加载工作流详情失败: ' + message
    }
  } finally {
    if (currentRequest === requestId) {
      loading.value = false
      refreshing.value = false
    }
  }
}

function phaseClass(phase) {
  return { Pending: 'pending', Running: 'running', Succeeded: 'succeeded', Failed: 'failed', Skipped: 'skipped' }[phase] || 'pending'
}

function formatTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit'
  })
}
</script>

<style scoped>
.header { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 12px; }
.refresh-controls { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; font-size: 12px; color: #64748b; }
.refresh-controls label { display: flex; align-items: center; gap: 5px; }
.back, td a { color: #6366f1; text-decoration: none; font-size: 13px; }
a:hover { text-decoration: underline; }
button { padding: 6px 10px; border: 1px solid #cbd5e1; border-radius: 6px; background: #fff; cursor: pointer; }
button:hover:not(:disabled) { background: #f1f5f9; }
button:disabled { opacity: .4; cursor: default; }
h1 { font-size: 22px; margin-bottom: 20px; overflow-wrap: anywhere; }
.card { background: #fff; border-radius: 8px; padding: 16px; margin-bottom: 16px; }
h2 { font-size: 14px; color: #64748b; margin-bottom: 12px; }
dl { display: grid; grid-template-columns: 80px minmax(0, 1fr); gap: 8px 12px; font-size: 13px; }
dt { color: #94a3b8; }
dd { color: #374151; overflow-wrap: anywhere; }
.message { white-space: pre-wrap; }
.loading, .empty { padding: 40px; text-align: center; color: #64748b; }
.error-msg, .warning { padding: 10px 14px; margin-bottom: 16px; border-radius: 6px; font-size: 13px; overflow-wrap: anywhere; white-space: pre-wrap; }
.error-msg { background: #fee2e2; color: #dc2626; }
.warning { background: #fef3c7; color: #92400e; }
.deleting { display: inline-block; margin-left: 6px; color: #d97706; }
.table-wrap { overflow-x: auto; }
table { width: 100%; min-width: 1050px; border-collapse: collapse; }
th, td { padding: 10px 12px; text-align: left; font-size: 13px; white-space: nowrap; }
th { color: #64748b; font-weight: 600; background: #f8fafc; border-bottom: 1px solid #e2e8f0; }
td { border-bottom: 1px solid #f1f5f9; }
.log-link { margin-left: 10px; }
.time-cell { color: #64748b; font-size: 12px; }
.message-cell { min-width: 160px; max-width: 280px; white-space: pre-wrap; overflow-wrap: anywhere; }
.tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.tag.pending { background: #f1f5f9; color: #64748b; }
.tag.running { background: #dbeafe; color: #1d4ed8; }
.tag.succeeded { background: #dcfce7; color: #16a34a; }
.tag.failed { background: #fee2e2; color: #dc2626; }
.tag.skipped { background: #fef3c7; color: #92400e; }
</style>
