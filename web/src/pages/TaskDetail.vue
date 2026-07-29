<template>
  <div class="detail">
    <div v-if="loading" class="loading">加载中...</div>
    <NoCluster v-else-if="clusterError" :message="clusterError" />
    <div v-else-if="error" class="error-msg">{{ error }}</div>

    <template v-else-if="task">
      <div class="header">
        <router-link to="/tasks" class="back">← 返回列表</router-link>
        <div class="header-actions">
          <button @click="doTrigger" :disabled="acting">▶ 触发</button>
          <button v-if="task.spec?.suspend" @click="doResume" :disabled="acting">↻ 恢复</button>
          <button v-else @click="doSuspend" :disabled="acting">⏸ 暂停</button>
          <router-link :to="'/tasks/' + task.metadata.name + '/logs'" class="btn-link">📋 查看日志</router-link>
          <button @click="doDelete" class="danger" :disabled="acting">✕ 删除</button>
        </div>
      </div>

      <h1>{{ task.metadata.name }}</h1>

      <div class="row">
        <!-- 基本信息 -->
        <div class="card">
          <h3>基本信息</h3>
          <dl>
            <dt>类型</dt><dd><span class="tag type">{{ task.spec?.type }}</span></dd>
            <dt>状态</dt><dd><span class="tag" :class="phaseClass(task.status?.phase)">{{ task.status?.phase || 'Pending' }}</span></dd>
            <dt>镜像</dt><dd><code>{{ task.spec?.image }}</code></dd>
            <dt v-if="task.spec?.command">命令</dt><dd v-if="task.spec?.command"><code>{{ task.spec.command.join(' ') }}</code></dd>
            <dt v-if="task.spec?.schedule">Cron</dt><dd v-if="task.spec?.schedule"><code>{{ task.spec.schedule }}</code></dd>
            <dt v-if="task.spec?.delay">延迟</dt><dd v-if="task.spec?.delay"><code>{{ task.spec.delay }}</code></dd>
            <dt>成功</dt><dd class="green">{{ task.status?.succeeded || 0 }}</dd>
            <dt>失败</dt><dd class="red">{{ task.status?.failed || 0 }}</dd>
            <dt>创建时间</dt><dd>{{ formatTime(task.metadata.creationTimestamp) }}</dd>
            <dt v-if="task.status?.lastStartTime">上次执行</dt><dd v-if="task.status?.lastStartTime">{{ formatTime(task.status.lastStartTime) }}</dd>
            <dt v-if="task.status?.lastCompletionTime">上次完成</dt><dd v-if="task.status?.lastCompletionTime">{{ formatTime(task.status.lastCompletionTime) }}</dd>
          </dl>
        </div>

        <!-- YAML 预览 -->
        <div class="card">
          <h3>YAML</h3>
          <pre class="yaml">{{ yamlPreview }}</pre>
        </div>
      </div>

      <!-- 执行历史 -->
      <div class="card" v-if="task.status?.executionHistory?.length">
        <h3>执行历史</h3>
        <table>
          <thead>
            <tr><th>Job</th><th>开始时间</th><th>结束时间</th><th>结果</th></tr>
          </thead>
          <tbody>
            <tr v-for="rec in task.status.executionHistory.slice().reverse()" :key="rec.jobName">
              <td><code>{{ rec.jobName }}</code></td>
              <td>{{ formatTime(rec.startTime) }}</td>
              <td>{{ formatTime(rec.stopTime) }}</td>
              <td><span class="tag" :class="phaseClass(rec.phase)">{{ rec.phase }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api'
import { useClusterStatus } from '@/composables/cluster'
import NoCluster from '@/components/NoCluster.vue'

const route = useRoute()
const router = useRouter()
const { clusterError, handleApiError } = useClusterStatus()

const loading = ref(true)
const error = ref('')
const acting = ref(false)
const task = ref(null)

const yamlPreview = computed(() => {
  if (!task.value) return ''
  return `apiVersion: kubetask.kubetask.io/v1
kind: Task
metadata:
  name: ${task.value.metadata.name}
  creationTimestamp: ${formatTime(task.value.metadata.creationTimestamp)}
spec:
  type: ${task.value.spec?.type}
  image: ${task.value.spec?.image}
  ${task.value.spec?.command ? 'command:\n' + task.value.spec.command.map(c => '    - ' + c).join('\n') : 'command: []'}
  backoffLimit: ${task.value.spec?.backoffLimit || 3}
status:
  phase: ${task.value.status?.phase || 'Pending'}
  succeeded: ${task.value.status?.succeeded || 0}
  failed: ${task.value.status?.failed || 0}`
})

onMounted(() => fetchTask())

async function fetchTask() {
  loading.value = true
  try {
    task.value = await api.getTask(route.params.name)
  } catch (e) {
    if (handleApiError(e) !== null) error.value = '加载失败: ' + e.message
  } finally {
    loading.value = false
  }
}

async function doAction(action) {
  acting.value = true
  try {
    await action()
    await fetchTask()
  } catch (e) {
    if (handleApiError(e) !== null) error.value = '操作失败: ' + e.message
  } finally {
    acting.value = false
  }
}
function doTrigger() { doAction(() => api.triggerTask(route.params.name)) }
function doSuspend() { doAction(() => api.suspendTask(route.params.name)) }
function doResume() { doAction(() => api.resumeTask(route.params.name)) }
function doDelete() {
  if (!confirm('确认删除？')) return
  doAction(async () => { await api.deleteTask(route.params.name); router.push('/tasks') })
}

function phaseClass(phase) {
  const map = { Running: 'running', Succeeded: 'succeeded', Failed: 'failed', Suspended: 'suspended', Pending: 'pending' }
  return map[phase] || 'pending'
}

function formatTime(t) {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
</script>

<style scoped>
.detail { animation: fadeIn .2s; }
@keyframes fadeIn { from { opacity: 0 } to { opacity: 1 } }
.loading { padding: 40px; text-align: center; color: #64748b; }
.error-msg { padding: 10px 14px; margin-bottom: 12px; background: #fee2e2; color: #dc2626; border-radius: 6px; font-size: 13px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
.back { color: #6366f1; font-size: 13px; text-decoration: none; }
.back:hover { text-decoration: underline; }
.header-actions { display: flex; gap: 6px; }
.header-actions button { padding: 5px 12px; border: 1px solid #e2e8f0; border-radius: 6px; cursor: pointer; font-size: 13px; background: #fff; }
.header-actions button:hover:not(:disabled) { background: #f1f5f9; }
.header-actions button:disabled { opacity: .4; cursor: default; }
.header-actions button.danger:hover:not(:disabled) { background: #fee2e2; color: #dc2626; border-color: #fecaca; }
.header-actions .btn-link { text-decoration: none; }
h1 { font-size: 22px; margin-bottom: 20px; }
.row { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-bottom: 16px; }
.card { background: #fff; border-radius: 8px; padding: 16px; }
.card h3 { font-size: 14px; color: #64748b; margin-bottom: 12px; }
dl { display: grid; grid-template-columns: 80px 1fr; gap: 6px 12px; font-size: 13px; }
dt { color: #94a3b8; }
dd { color: #374151; word-break: break-all; }
dd code { font-size: 12px; background: #f1f5f9; padding: 1px 4px; border-radius: 3px; }
dd.green { color: #16a34a; font-weight: 600; }
dd.red { color: #dc2626; font-weight: 600; }
.tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
.tag.type { background: #e0e7ff; color: #4338ca; }
.tag.running { background: #dbeafe; color: #1d4ed8; }
.tag.succeeded { background: #dcfce7; color: #16a34a; }
.tag.failed { background: #fee2e2; color: #dc2626; }
.tag.suspended { background: #fef3c7; color: #d97706; }
.tag.pending { background: #f1f5f9; color: #64748b; }
.yaml { font-size: 12px; font-family: monospace; color: #374151; white-space: pre; overflow-x: auto; line-height: 1.6; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 8px 10px; text-align: left; font-size: 12px; }
th { color: #94a3b8; font-weight: 600; border-bottom: 1px solid #e5e7eb; }
td { border-bottom: 1px solid #f1f5f9; }
td code { font-size: 11px; background: #f1f5f9; padding: 1px 4px; border-radius: 3px; }
</style>
