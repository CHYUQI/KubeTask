import { beginAuthRequest } from './auth.js'
import { API_BASE, REQUEST_TIMEOUT, checkAPIResponse } from './http.js'

async function request(url, options = {}) {
  const scope = beginAuthRequest()
  const { controller } = scope
  let timedOut = false
  const timer = setTimeout(() => { timedOut = true; controller.abort() }, REQUEST_TIMEOUT)

  try {
    const res = await fetch(API_BASE + url, {
      ...options,
      headers: { 'Content-Type': 'application/json', ...options.headers, ...scope.headers },
      signal: controller.signal,
      redirect: 'error',
      cache: 'no-store'
    })
    controller.signal.throwIfAborted()
    await checkAPIResponse(res, scope)

    const text = await res.text()
    controller.signal.throwIfAborted()
    if (!text) return {}
    try { return JSON.parse(text) } catch (_) {
      throw new Error('非 JSON 响应: ' + text.slice(0, 100))
    }
  } catch (e) {
    if (timedOut) throw new Error('请求超时')
    throw e
  } finally {
    clearTimeout(timer)
    controller.abort()
    scope.release()
  }
}

export const api = {
  listWorkflows(params = {}) {
    const q = new URLSearchParams(params).toString()
    return request('/workflows' + (q ? '?' + q : ''))
  },
  getWorkflow(name) { return request('/workflows/' + encodeURIComponent(name)) },
  createWorkflow(data) { return request('/workflows', { method: 'POST', body: JSON.stringify(data) }) },
  listTasks(params = {}) {
    const q = new URLSearchParams(params).toString()
    return request('/tasks' + (q ? '?' + q : ''))
  },
  getTask(name) { return request('/tasks/' + encodeURIComponent(name)) },
  createTask(data) { return request('/tasks', { method: 'POST', body: JSON.stringify(data) }) },
  updateTask(name, data) { return request('/tasks/' + encodeURIComponent(name), { method: 'PUT', body: JSON.stringify(data) }) },
  deleteTask(name) { return request('/tasks/' + encodeURIComponent(name), { method: 'DELETE' }) },
  triggerTask(name) { return request('/tasks/' + encodeURIComponent(name) + '/trigger', { method: 'POST' }) },
  suspendTask(name) { return request('/tasks/' + encodeURIComponent(name) + '/suspend', { method: 'POST' }) },
  resumeTask(name) { return request('/tasks/' + encodeURIComponent(name) + '/resume', { method: 'POST' }) },
  getStats() { return request('/stats') },
  getTrend() { return request('/stats/trend') }
}
