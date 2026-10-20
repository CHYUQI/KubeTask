import { beginAuthRequest } from './auth.js'
import { API_BASE, REQUEST_TIMEOUT, checkAPIResponse } from './http.js'

// 原生 EventSource 不支持 Authorization 请求头，用 fetch 保持原有 SSE 协议。
export async function streamTaskLogs(name, { tail, follow, signal, onMessage }) {
  const scope = beginAuthRequest()
  const { controller } = scope
  const abort = () => controller.abort()
  signal?.addEventListener('abort', abort, { once: true })
  if (signal?.aborted) abort()
  let timedOut = false
  // 只限制建立连接，不截断长时间运行的日志流。
  const timer = setTimeout(() => { timedOut = true; abort() }, REQUEST_TIMEOUT)
  try {
    const query = new URLSearchParams({ tail, follow })
    const response = await fetch(`${API_BASE}/tasks/${encodeURIComponent(name)}/logs?${query}`, {
      headers: { Accept: 'text/event-stream', ...scope.headers },
      signal: controller.signal,
      redirect: 'error',
      cache: 'no-store'
    })
    controller.signal.throwIfAborted()
    await checkAPIResponse(response, scope)
    if (response.headers.get('Content-Type')?.split(';')[0].trim().toLowerCase() !== 'text/event-stream' || !response.body) {
      throw new Error('日志服务未返回 SSE 日志流')
    }
    clearTimeout(timer)
    await readSSE(response.body, controller.signal, onMessage)
    // Pod 自然结束时，follow=true 的日志响应也可能正常 EOF。
  } catch (error) {
    if (timedOut) throw new Error('日志流连接超时')
    throw error
  } finally {
    clearTimeout(timer)
    controller.abort()
    signal?.removeEventListener('abort', abort)
    scope.release()
  }
}

async function readSSE(body, signal, onMessage) {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let data = []
  let event = ''
  const cancel = () => { reader.cancel().catch(() => {}) }
  signal.addEventListener('abort', cancel, { once: true })

  function line(value) {
    if (value === '') {
      if (data.length && (!event || event === 'message')) onMessage(data.join('\n'))
      data = []
      event = ''
      return
    }
    const colon = value.indexOf(':')
    const field = colon === -1 ? value : value.slice(0, colon)
    let content = colon === -1 ? '' : value.slice(colon + 1)
    if (content.startsWith(' ')) content = content.slice(1)
    if (field === 'data') data.push(content)
    if (field === 'event') event = content
  }

  function drain(final = false) {
    let start = 0
    for (let i = 0; i < buffer.length; i++) {
      if (buffer[i] !== '\r' && buffer[i] !== '\n') continue
      // CRLF 可能横跨两个网络分块，先保留末尾的 CR。
      if (buffer[i] === '\r' && i === buffer.length - 1 && !final) break
      signal.throwIfAborted()
      line(buffer.slice(start, i))
      if (buffer[i] === '\r' && buffer[i + 1] === '\n') i++
      start = i + 1
    }
    buffer = buffer.slice(start)
  }

  try {
    while (true) {
      signal.throwIfAborted()
      const { value, done } = await reader.read()
      signal.throwIfAborted()
      buffer += decoder.decode(value, { stream: !done })
      drain(done)
      if (done) return // 未以空行结束的残缺事件按 SSE 规则丢弃。
    }
  } finally {
    signal.removeEventListener('abort', cancel)
    try { await reader.cancel() } catch { /* 网络错误不应覆盖原始错误。 */ }
    reader.releaseLock()
  }
}
