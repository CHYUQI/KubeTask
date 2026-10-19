<template>
  <div class="workflow-list">
    <h1>工作流列表</h1>

    <div class="filters">
      <select v-model="filterPhase" :disabled="loading" aria-label="工作流状态" @change="fetchWorkflows(1)">
        <option value="">全部状态</option>
        <option value="Pending">Pending</option>
        <option value="Running">Running</option>
        <option value="Succeeded">Succeeded</option>
        <option value="Failed">Failed</option>
      </select>
      <button :disabled="loading" @click="fetchWorkflows(page)">刷新</button>
    </div>

    <div v-if="loading" class="loading">加载中...</div>
    <NoCluster v-else-if="clusterError" :message="clusterError" />
    <div v-else-if="error" class="error-msg" role="alert">{{ error }}</div>
    <div v-else>
      <div v-if="workflows.length > 0" class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>名称</th>
              <th>状态</th>
              <th>节点数</th>
              <th>并行上限</th>
              <th>创建时间</th>
              <th>开始时间</th>
              <th>完成时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="wf in workflows" :key="wf.metadata.name">
              <td>
                <router-link :to="{ name: 'WorkflowDetail', params: { name: wf.metadata.name } }">{{ wf.metadata.name }}</router-link>
              </td>
              <td>
                <span class="tag" :class="phaseClass(wf.status?.phase)">{{ wf.status?.phase || '未初始化' }}</span>
                <span v-if="wf.metadata.deletionTimestamp" class="deleting">删除中</span>
              </td>
              <td>{{ wf.spec?.tasks?.length || 0 }}</td>
              <td>{{ wf.spec?.maxParallel || '不限' }}</td>
              <td class="time-cell">{{ formatTime(wf.metadata.creationTimestamp) }}</td>
              <td class="time-cell">{{ formatTime(wf.status?.startTime) }}</td>
              <td class="time-cell">{{ formatTime(wf.status?.completionTime) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else class="empty">{{ filterPhase ? '没有符合当前状态的工作流' : '暂无工作流' }}</div>

      <div v-if="total > PAGE_SIZE" class="pager">
        <button :disabled="page <= 1" @click="fetchWorkflows(page - 1)">上一页</button>
        <span>第 {{ page }} / {{ totalPages }} 页 (共 {{ total }} 条)</span>
        <button :disabled="page >= totalPages" @click="fetchWorkflows(page + 1)">下一页</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '@/api'
import { useClusterStatus } from '@/composables/cluster'
import NoCluster from '@/components/NoCluster.vue'

const PAGE_SIZE = 10
const { clusterError, handleApiError } = useClusterStatus()
const loading = ref(true)
const error = ref('')
const workflows = ref([])
const filterPhase = ref('')
const page = ref(1)
const total = ref(0)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))

onMounted(() => fetchWorkflows(1))

async function fetchWorkflows(p) {
  loading.value = true
  error.value = ''
  clusterError.value = ''
  try {
    const params = { page: p, pageSize: PAGE_SIZE }
    if (filterPhase.value) params.phase = filterPhase.value
    const data = await api.listWorkflows(params)
    const lastPage = Math.max(1, Math.ceil(data.total / PAGE_SIZE))
    // 工作流被删除后，刷新时返回仍有数据的最后一页。
    if (p > lastPage) {
      await fetchWorkflows(lastPage)
      return
    }
    workflows.value = data.items || []
    total.value = data.total || 0
    page.value = data.page || p
  } catch (e) {
    const message = handleApiError(e)
    if (message !== null) error.value = '加载工作流列表失败: ' + message
  } finally {
    loading.value = false
  }
}

function phaseClass(phase) {
  return { Pending: 'pending', Running: 'running', Succeeded: 'succeeded', Failed: 'failed' }[phase] || 'pending'
}

function formatTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'
  })
}
</script>

<style scoped>
.workflow-list h1 { font-size: 22px; margin-bottom: 16px; }
.filters { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.filters select, button { padding: 6px 10px; border: 1px solid #cbd5e1; border-radius: 6px; font-size: 13px; background: #fff; }
button { cursor: pointer; }
button:hover:not(:disabled) { background: #f1f5f9; }
button:disabled, select:disabled { opacity: .4; cursor: default; }
.loading, .empty { padding: 40px; text-align: center; color: #64748b; }
.error-msg { padding: 10px 14px; background: #fee2e2; color: #dc2626; border-radius: 6px; font-size: 13px; }
.table-wrap { overflow-x: auto; border-radius: 8px; }
table { width: 100%; min-width: 850px; border-collapse: collapse; background: #fff; }
th, td { padding: 10px 14px; text-align: left; font-size: 13px; }
th { background: #f8fafc; color: #64748b; font-weight: 600; border-bottom: 1px solid #e2e8f0; white-space: nowrap; }
td { border-bottom: 1px solid #f1f5f9; white-space: nowrap; }
td a { color: #6366f1; text-decoration: none; }
td a:hover { text-decoration: underline; }
.tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.tag.pending { background: #f1f5f9; color: #64748b; }
.tag.running { background: #dbeafe; color: #1d4ed8; }
.tag.succeeded { background: #dcfce7; color: #16a34a; }
.tag.failed { background: #fee2e2; color: #dc2626; }
.deleting { display: inline-block; margin-left: 6px; color: #d97706; font-size: 12px; }
.time-cell { font-size: 12px; color: #64748b; white-space: nowrap; }
.pager { display: flex; justify-content: center; align-items: center; gap: 14px; margin-top: 16px; font-size: 13px; color: #64748b; }
</style>
