<template>
  <div class="logs">
    <div class="header">
      <router-link :to="'/tasks/' + taskName" class="back">← 返回详情</router-link>
      <span class="title">日志: <strong>{{ taskName }}</strong></span>
      <div class="controls">
        <label>行数 <input v-model.number="tail" type="number" min="1" max="10000" /></label>
        <label class="follow-label">
          <input v-model="follow" type="checkbox" />
          实时跟踪
        </label>
        <button @click="toggleStream" :class="{ active: streaming }">
          {{ streaming ? '⏹ 停止' : '▶ 开始' }}
        </button>
      </div>
    </div>

    <!-- 搜索栏 -->
    <div v-if="lines.length > 0" class="search-bar">
      <input v-model="searchQuery" placeholder="搜索日志..." />
      <span v-if="searchQuery" class="match-info">
        {{ filteredLines.length }}/{{ lines.length }} 条匹配
      </span>
      <button v-if="searchQuery" @click="searchQuery = ''" class="clear-btn">✕</button>
    </div>

    <NoCluster v-if="clusterError" :message="clusterError" />
    <div v-else-if="error" class="error-msg">{{ error }}</div>

    <div v-else ref="logContainer" class="log-container" @scroll="onScroll">
      <div v-if="!streaming && lines.length === 0" class="hint">点击「开始」加载日志</div>
      <div v-for="item in filteredLines" :key="item.index" class="log-line" :class="lineClass(item.text)">
        <span class="line-no">{{ item.index + 1 }}</span>
        <span class="line-text" v-html="highlightText(item.text)"></span>
      </div>
      <div v-if="searchQuery && filteredLines.length === 0 && lines.length > 0" class="hint">无匹配结果</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useClusterStatus } from '@/composables/cluster'
import NoCluster from '@/components/NoCluster.vue'

const route = useRoute()
const taskName = route.params.name
const { clusterError, handleApiError } = useClusterStatus()

const error = ref('')
const streaming = ref(false)
const tail = ref(200)
const follow = ref(false)
const lines = ref([])
const searchQuery = ref('')
const logContainer = ref(null)
const autoScroll = ref(true)

let eventSource = null

const filteredLines = computed(() => {
  if (!searchQuery.value) return lines.value.map((text, index) => ({ index, text }))
  const q = searchQuery.value.toLowerCase()
  return lines.value
    .map((text, index) => ({ index, text }))
    .filter(item => item.text.toLowerCase().includes(q))
})

function escapeHtml(text) {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightText(text) {
  const escaped = escapeHtml(text)
  if (!searchQuery.value) return escaped
  const q = searchQuery.value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return escaped.replace(new RegExp(`(${q})`, 'gi'), '<mark>$1</mark>')
}

function toggleStream() {
  if (streaming.value) {
    stopStream()
  } else {
    startStream()
  }
}

function startStream() {
  error.value = ''
  searchQuery.value = ''
  lines.value = []
  streaming.value = true

  const url = `/api/v1/tasks/${taskName}/logs?tail=${tail.value}&follow=${follow.value}`
  eventSource = new EventSource(url)

  eventSource.onmessage = (e) => {
    if (e.data) lines.value.push(e.data)
    if (autoScroll.value) scrollBottom()
  }

  eventSource.onerror = () => {
    error.value = '日志流连接中断'
    stopStream()
  }
}

function stopStream() {
  streaming.value = false
  eventSource?.close()
  eventSource = null
}

function scrollBottom() {
  const el = logContainer.value
  if (el) el.scrollTop = el.scrollHeight
}

function onScroll() {
  if (!logContainer.value) return
  const el = logContainer.value
  autoScroll.value = el.scrollHeight - el.scrollTop - el.clientHeight < 50
}

function lineClass(line) {
  const lower = line.toLowerCase()
  if (lower.includes('error') || lower.includes('fail') || lower.includes('fatal')) return 'level-error'
  if (lower.includes('warn')) return 'level-warn'
  return ''
}

onUnmounted(() => stopStream())
</script>

<style scoped>
.logs { animation: fadeIn .2s; }
@keyframes fadeIn { from { opacity: 0 } to { opacity: 1 } }
.header { display: flex; align-items: center; gap: 14px; margin-bottom: 14px; }
.back { color: #6366f1; font-size: 13px; text-decoration: none; flex-shrink: 0; }
.back:hover { text-decoration: underline; }
.title { font-size: 14px; }
.controls { display: flex; gap: 10px; align-items: center; margin-left: auto; font-size: 13px; }
.controls label { display: flex; align-items: center; gap: 4px; color: #64748b; }
.controls input[type="number"] { width: 60px; padding: 3px 6px; border: 1px solid #d1d5db; border-radius: 4px; }
.controls button { padding: 4px 14px; border: 1px solid #cbd5e1; border-radius: 6px; cursor: pointer; font-size: 13px; background: #fff; }
.controls button:hover { background: #f1f5f9; }
.controls button.active { background: #dc2626; color: #fff; border-color: #dc2626; }
.error-msg { padding: 10px 14px; margin-bottom: 12px; background: #fee2e2; color: #dc2626; border-radius: 6px; font-size: 13px; }
.search-bar { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.search-bar input { flex: 1; max-width: 300px; padding: 6px 10px; border: 1px solid #cbd5e1; border-radius: 6px; font-size: 13px; background: #fff; }
.match-info { font-size: 12px; color: #64748b; white-space: nowrap; }
.clear-btn { padding: 2px 8px; border: 1px solid #e2e8f0; border-radius: 4px; cursor: pointer; font-size: 12px; background: #fff; }
.clear-btn:hover { background: #f1f5f9; }
.log-container { background: #1e293b; color: #e2e8f0; border-radius: 8px; padding: 14px; height: calc(100vh - 200px); overflow-y: auto; font-family: 'Consolas', 'Courier New', monospace; font-size: 13px; line-height: 1.6; }
.hint { padding: 40px; text-align: center; color: #64748b; }
.log-line { display: flex; gap: 14px; }
.log-line:hover { background: rgba(255,255,255,.05); }
.line-no { color: #475569; min-width: 40px; text-align: right; user-select: none; }
.line-text { flex: 1; white-space: pre-wrap; word-break: break-all; }
.level-error { background: rgba(239,68,68,.15); }
.level-warn { background: rgba(245,158,11,.1); }
.line-text :deep(mark) { background: rgba(250,204,21,.4); color: #facc15; border-radius: 2px; }
</style>
