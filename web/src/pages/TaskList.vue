<template>
  <div class="task-list">
    <h1>任务列表</h1>

    <div v-if="loading" class="loading">加载中...</div>
    <NoCluster v-else-if="clusterError" :message="clusterError" />

    <div v-else>
      <div class="filters">
        <select v-model="filterType" @change="fetchTasks(1)">
          <option value="">全部类型</option>
          <option value="Cron">Cron</option>
          <option value="OneTime">OneTime</option>
          <option value="Delay">Delay</option>
        </select>
        <select v-model="filterPhase" @change="fetchTasks(1)">
          <option value="">全部状态</option>
          <option value="Pending">Pending</option>
          <option value="Running">Running</option>
          <option value="Succeeded">Succeeded</option>
          <option value="Failed">Failed</option>
          <option value="Suspended">Suspended</option>
        </select>
        <router-link to="/tasks/create" class="btn-create">+ 创建任务</router-link>
      </div>

      <div v-if="error" class="error-msg">{{ error }}</div>

      <table v-if="tasks.length > 0">
        <thead>
          <tr>
            <th>名称</th>
            <th>类型</th>
            <th>状态</th>
            <th>镜像</th>
            <th>上次执行</th>
            <th>成功/失败</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in tasks" :key="t.metadata.name">
            <td>
              <router-link :to="'/tasks/' + t.metadata.name">{{ t.metadata.name }}</router-link>
            </td>
            <td><span class="tag type">{{ t.spec?.type }}</span></td>
            <td><span class="tag" :class="phaseClass(t.status?.phase)">{{ t.status?.phase || 'Pending' }}</span></td>
            <td class="img-cell">{{ t.spec?.image }}</td>
            <td class="time-cell">{{ formatTime(t.status?.lastStartTime) }}</td>
            <td class="count-cell">
              <span class="green">{{ t.status?.succeeded || 0 }}</span>
              /
              <span class="red">{{ t.status?.failed || 0 }}</span>
            </td>
            <td class="actions">
              <button :disabled="acting === t.metadata.name" @click="doTrigger(t)" title="触发">▶</button>
              <button v-if="t.spec?.suspend" :disabled="acting === t.metadata.name" @click="doResume(t)" title="恢复">↻</button>
              <button v-else :disabled="acting === t.metadata.name" @click="doSuspend(t)" title="暂停">⏸</button>
              <router-link :to="'/tasks/' + t.metadata.name + '/logs'" class="btn-link" title="日志">📋</router-link>
              <button :disabled="acting === t.metadata.name" @click="doDelete(t)" class="danger" title="删除">✕</button>
            </td>
          </tr>
        </tbody>
      </table>

      <div v-else class="empty">暂无任务，<router-link to="/tasks/create">创建一个</router-link></div>

      <div class="pager" v-if="total > PAGE_SIZE">
        <button :disabled="page <= 1" @click="fetchTasks(page - 1)">上一页</button>
        <span>第 {{ page }} / {{ totalPages }} 页 (共 {{ total }} 条)</span>
        <button :disabled="page >= totalPages" @click="fetchTasks(page + 1)">下一页</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '@/api'
import { useClusterStatus } from '@/composables/cluster'
import NoCluster from '@/components/NoCluster.vue'

const { clusterError, handleApiError } = useClusterStatus()

const PAGE_SIZE = 10

const loading = ref(true)
const error = ref('')
const acting = ref('')
const tasks = ref([])
const page = ref(1)
const total = ref(0)
const filterType = ref('')
const filterPhase = ref('')

const totalPages = computed(() => Math.ceil(total.value / PAGE_SIZE))

onMounted(() => fetchTasks(1))

async function fetchTasks(p) {
  loading.value = true
  error.value = ''
  try {
    const params = { page: p, pageSize: PAGE_SIZE }
    if (filterType.value) params.type = filterType.value
    if (filterPhase.value) params.phase = filterPhase.value
    const data = await api.listTasks(params)
    tasks.value = data.items || []
    total.value = data.total || 0
    page.value = data.page || p
  } catch (e) {
    if (handleApiError(e) !== null) {
      error.value = '加载任务列表失败: ' + handleApiError(e)
    }
  } finally {
    loading.value = false
  }
}

async function doAction(t, action) {
  acting.value = t.metadata.name
  try {
    await action(t)
    await fetchTasks(page.value)
  } catch (e) {
    error.value = '操作失败: ' + e.message
  } finally {
    acting.value = ''
  }
}

function doTrigger(t) {
  if (!confirm('确认触发 ' + t.metadata.name + '？')) return
  doAction(t, t => api.triggerTask(t.metadata.name))
}
function doSuspend(t) { doAction(t, t => api.suspendTask(t.metadata.name)) }
function doResume(t) { doAction(t, t => api.resumeTask(t.metadata.name)) }
function doDelete(t) {
  if (!confirm('确认删除 ' + t.metadata.name + '？')) return
  doAction(t, t => api.deleteTask(t.metadata.name))
}

function phaseClass(phase) {
  const map = { Running: 'running', Succeeded: 'succeeded', Failed: 'failed', Suspended: 'suspended', Pending: 'pending' }
  return map[phase] || 'pending'
}

function formatTime(t) {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}
</script>

<style scoped>
.task-list h1 { font-size: 22px; margin-bottom: 16px; }
.loading { padding: 40px; text-align: center; color: #64748b; }
.filters { display: flex; gap: 10px; margin-bottom: 16px; align-items: center; }
.filters select { padding: 6px 10px; border: 1px solid #cbd5e1; border-radius: 6px; font-size: 13px; background: #fff; }
.btn-create { margin-left: auto; padding: 6px 16px; background: #6366f1; color: #fff; border-radius: 6px; text-decoration: none; font-size: 13px; }
.btn-create:hover { background: #4f46e5; }
.error-msg { padding: 10px 14px; margin-bottom: 12px; background: #fee2e2; color: #dc2626; border-radius: 6px; font-size: 13px; }
table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; }
th, td { padding: 10px 14px; text-align: left; font-size: 13px; }
th { background: #f8fafc; color: #64748b; font-weight: 600; border-bottom: 1px solid #e2e8f0; }
td { border-bottom: 1px solid #f1f5f9; }
td a { color: #6366f1; text-decoration: none; }
td a:hover { text-decoration: underline; }
.tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.tag.type { background: #e0e7ff; color: #4338ca; }
.tag.running { background: #dbeafe; color: #1d4ed8; }
.tag.succeeded { background: #dcfce7; color: #16a34a; }
.tag.failed { background: #fee2e2; color: #dc2626; }
.tag.suspended { background: #fef3c7; color: #d97706; }
.tag.pending { background: #f1f5f9; color: #64748b; }
.img-cell { font-family: monospace; font-size: 12px; max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.time-cell { font-size: 12px; color: #64748b; white-space: nowrap; }
.count-cell { font-size: 12px; }
.count-cell .green { color: #16a34a; } .count-cell .red { color: #dc2626; }
.actions { white-space: nowrap; }
.actions button { padding: 3px 7px; border: 1px solid #e2e8f0; border-radius: 4px; cursor: pointer; font-size: 12px; background: #fff; margin-right: 4px; }
.actions button:hover:not(:disabled) { background: #f1f5f9; }
.actions button:disabled { opacity: .4; cursor: default; }
.actions button.danger:hover:not(:disabled) { background: #fee2e2; color: #dc2626; border-color: #fecaca; }
.btn-link { text-decoration: none; font-size: 12px; margin-right: 4px; }
.empty { text-align: center; padding: 60px; color: #94a3b8; font-size: 14px; }
.empty a { color: #6366f1; }
.pager { display: flex; justify-content: center; align-items: center; gap: 14px; margin-top: 16px; font-size: 13px; color: #64748b; }
.pager button { padding: 4px 14px; border: 1px solid #cbd5e1; border-radius: 6px; background: #fff; cursor: pointer; font-size: 13px; }
.pager button:disabled { opacity: .4; cursor: default; }
</style>
