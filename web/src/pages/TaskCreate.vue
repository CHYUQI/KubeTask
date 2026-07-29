<template>
  <div class="create-task">
    <h1>创建任务</h1>
    <div v-if="error" class="error-msg">{{ error }}</div>
    <NoCluster v-if="clusterError" :message="clusterError" />

    <form v-if="!clusterError" @submit.prevent="submit" class="form">
      <div class="form-group">
        <label>名称 <span class="opt">(可选，留空自动生成)</span></label>
        <input v-model="form.name" placeholder="my-task" />
      </div>

      <div class="form-group">
        <label>类型</label>
        <select v-model="form.type">
          <option value="OneTime">OneTime — 一次性执行</option>
          <option value="Cron">Cron — 定时执行</option>
          <option value="Delay">Delay — 延迟执行</option>
        </select>
      </div>

      <div class="form-group" v-if="form.type === 'Cron'">
        <label>Cron 表达式</label>
        <input v-model="form.schedule" placeholder="*/5 * * * *" />
        <p class="hint">标准 5 字段：分 时 日 月 周</p>
      </div>

      <div class="form-group" v-if="form.type === 'Cron'">
        <label>并发策略</label>
        <select v-model="form.concurrencyPolicy">
          <option value="Allow">Allow — 允许并发</option>
          <option value="Forbid">Forbid — 上一轮未结束则跳过</option>
          <option value="Replace">Replace — 替换旧的执行</option>
        </select>
      </div>

      <div class="form-group" v-if="form.type === 'Delay'">
        <label>延迟时长</label>
        <input v-model="form.delay" placeholder="5m" />
        <p class="hint">格式：30s / 5m / 1h</p>
      </div>

      <div class="form-group">
        <label>镜像 <span class="req">*</span></label>
        <input v-model="form.image" placeholder="busybox:latest" required />
      </div>

      <div class="form-row">
        <div class="form-group">
          <label>命令 Command</label>
          <input v-model="form.command" placeholder="echo,hello" />
        </div>
        <div class="form-group">
          <label>参数 Args</label>
          <input v-model="form.args" placeholder="arg1,arg2" />
        </div>
      </div>

      <div class="form-group">
        <label>环境变量 <span class="opt">(key=value, 每行一个)</span></label>
        <textarea v-model="form.envText" rows="3" placeholder="ENV=production&#10;LOG_LEVEL=debug"></textarea>
      </div>

      <h3>高级选项</h3>

      <div class="form-row">
        <div class="form-group">
          <label>失败重试次数</label>
          <input v-model.number="form.backoffLimit" type="number" min="0" />
        </div>
        <div class="form-group">
          <label>超时时间 (秒)</label>
          <input v-model.number="form.activeDeadlineSeconds" type="number" min="0" />
        </div>
        <div class="form-group">
          <label>完成后保留 (秒)</label>
          <input v-model.number="form.ttlSecondsAfterFinished" type="number" min="0" />
        </div>
      </div>

      <button type="submit" class="btn-submit" :disabled="submitting">
        {{ submitting ? '创建中...' : '创建任务' }}
      </button>
    </form>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { useClusterStatus } from '@/composables/cluster'
import NoCluster from '@/components/NoCluster.vue'

const router = useRouter()
const { clusterError, handleApiError } = useClusterStatus()
const error = ref('')
const submitting = ref(false)

const form = reactive({
  name: '',
  type: 'OneTime',
  schedule: '',
  concurrencyPolicy: 'Allow',
  delay: '',
  image: '',
  command: '',
  args: '',
  envText: '',
  backoffLimit: 3,
  activeDeadlineSeconds: null,
  ttlSecondsAfterFinished: 60
})

async function submit() {
  if (!form.image.trim()) {
    error.value = '镜像为必填字段'
    return
  }
  if (form.type === 'Cron' && !form.schedule.trim()) {
    error.value = 'Cron 类型需要填写 Cron 表达式'
    return
  }
  if (form.type === 'Delay' && !form.delay.trim()) {
    error.value = 'Delay 类型需要填写延迟时长'
    return
  }

  submitting.value = true
  error.value = ''

  try {
    const spec = {
      type: form.type,
      image: form.image.trim(),
      command: parseList(form.command),
      args: parseList(form.args),
      backoffLimit: form.backoffLimit || 3
    }

    if (form.type === 'Cron') {
      spec.schedule = form.schedule.trim()
      spec.concurrencyPolicy = form.concurrencyPolicy
    }
    if (form.type === 'Delay') spec.delay = form.delay.trim()
    if (form.activeDeadlineSeconds) spec.activeDeadlineSeconds = form.activeDeadlineSeconds
    if (form.ttlSecondsAfterFinished) spec.ttlSecondsAfterFinished = form.ttlSecondsAfterFinished

    const envLines = form.envText.trim().split('\n').filter(Boolean)
    if (envLines.length > 0) {
      spec.env = envLines.map(line => {
        const [name, ...rest] = line.split('=')
        return { name: name.trim(), value: rest.join('=').trim() }
      })
    }

    const body = {
      metadata: form.name.trim() ? { name: form.name.trim() } : {},
      spec
    }

    await api.createTask(body)
    router.push('/tasks')
  } catch (e) {
    if (handleApiError(e) !== null) {
      error.value = '创建失败: ' + handleApiError(e)
    }
  } finally {
    submitting.value = false
  }
}

function parseList(s) {
  if (!s || !s.trim()) return null
  return s.split(',').map(x => x.trim()).filter(Boolean)
}
</script>

<style scoped>
.create-task h1 { font-size: 22px; margin-bottom: 20px; }
.error-msg { padding: 10px 14px; margin-bottom: 16px; background: #fee2e2; color: #dc2626; border-radius: 6px; font-size: 13px; }
.form { background: #fff; border-radius: 8px; padding: 24px; max-width: 680px; }
.form-group { margin-bottom: 16px; display: flex; flex-direction: column; }
.form-group label { font-size: 13px; font-weight: 600; color: #374151; margin-bottom: 4px; }
.form-group .req { color: #ef4444; }
.form-group .opt { color: #94a3b8; font-weight: 400; font-size: 12px; }
.form-group input, .form-group select, .form-group textarea {
  padding: 8px 12px; border: 1px solid #d1d5db; border-radius: 6px; font-size: 13px;
  font-family: system-ui, monospace;
}
.form-group textarea { resize: vertical; }
.hint { font-size: 12px; color: #94a3b8; margin-top: 2px; }
.form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
.form-row:last-of-type { grid-template-columns: 1fr 1fr 1fr; }
h3 { font-size: 15px; color: #374151; margin: 24px 0 14px; padding-top: 16px; border-top: 1px solid #e5e7eb; }
.btn-submit {
  margin-top: 20px; padding: 10px 24px; background: #6366f1; color: #fff;
  border: none; border-radius: 6px; font-size: 14px; cursor: pointer;
}
.btn-submit:hover { background: #4f46e5; }
.btn-submit:disabled { opacity: .5; cursor: default; }
</style>
