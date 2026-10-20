import { computed, readonly, ref } from 'vue'

// 凭据只在当前页面内存中保留，不写入 URL 或浏览器持久化存储。
const token = ref('')
const revision = ref(0)
const required = ref(false)
const requests = new Set()

export const authRevision = readonly(revision)
export const authRequired = readonly(required)
export const hasAuthToken = computed(() => token.value !== '')

export function setAuthToken(value) {
  const next = value.trim()
  if (next && !/^[a-zA-Z0-9._~+/-]+=*$/.test(next)) {
    throw new Error('请输入原始 Token，不要包含 Bearer 前缀或空白字符')
  }
  for (const controller of requests) controller.abort()
  requests.clear()
  token.value = next
  required.value = false
  revision.value++
}

// 每个请求捕获自己的凭据版本；更换 Token 后，迟到的响应不能影响新会话。
export function beginAuthRequest() {
  const version = revision.value
  const controller = new AbortController()
  requests.add(controller)
  return {
    controller,
    headers: token.value ? { Authorization: `Bearer ${token.value}` } : {},
    requireAuthentication() {
      if (version === revision.value && !controller.signal.aborted) required.value = true
    },
    release() { requests.delete(controller) }
  }
}
