const BASE = '/api/v1'
const TIMEOUT = 10000

async function request(url, options = {}) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), TIMEOUT)

  try {
    const res = await fetch(BASE + url, {
      headers: { 'Content-Type': 'application/json', ...options.headers },
      signal: controller.signal,
      ...options
    })

    if (!res.ok) {
      let msg = res.statusText
      try { const e = await res.json(); msg = e.error || msg } catch (_) {}
      throw new Error(msg)
    }

    const text = await res.text()
    if (!text) return {}
    try { return JSON.parse(text) } catch (_) {
      throw new Error('非 JSON 响应: ' + text.slice(0, 100))
    }
  } catch (e) {
    if (e.name === 'AbortError') throw new Error('请求超时')
    throw e
  } finally {
    clearTimeout(timer)
  }
}

export const api = {
  listTasks(params = {}) {
    const q = new URLSearchParams(params).toString()
    return request('/tasks' + (q ? '?' + q : ''))
  },
  getTask(name) { return request('/tasks/' + name) },
  createTask(data) { return request('/tasks', { method: 'POST', body: JSON.stringify(data) }) },
  updateTask(name, data) { return request('/tasks/' + name, { method: 'PUT', body: JSON.stringify(data) }) },
  deleteTask(name) { return request('/tasks/' + name, { method: 'DELETE' }) },
  triggerTask(name) { return request('/tasks/' + name + '/trigger', { method: 'POST' }) },
  suspendTask(name) { return request('/tasks/' + name + '/suspend', { method: 'POST' }) },
  resumeTask(name) { return request('/tasks/' + name + '/resume', { method: 'POST' }) },
  getStats() { return request('/stats') },
  getTrend() { return request('/stats/trend') }
}
