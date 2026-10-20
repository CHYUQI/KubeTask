import test, { afterEach, beforeEach, mock } from 'node:test'
import assert from 'node:assert/strict'
import { api } from '../src/api/index.js'
import { authRequired, authRevision, hasAuthToken, setAuthToken } from '../src/api/auth.js'
import { streamTaskLogs } from '../src/api/logs.js'

beforeEach(() => setAuthToken(''))
afterEach(() => { setAuthToken(''); mock.restoreAll() })

function json(body, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function sse(body) {
  return new Response(body, { headers: { 'Content-Type': 'text/event-stream; charset=utf-8' } })
}

test('anonymous mode, setting, replacing and clearing Token affect every business request', async () => {
  const calls = []
  mock.method(globalThis, 'fetch', async (url, options) => {
    calls.push({ url, options })
    return json({})
  })
  await api.listTasks()
  assert.equal(calls.at(-1).options.headers.Authorization, undefined)

  setAuthToken('  opaque-token  ')
  assert.equal(hasAuthToken.value, true)
  const requests = [
    () => api.listTasks(), () => api.getTask('task'), () => api.createTask({ spec: {} }),
    () => api.updateTask('task', {}), () => api.deleteTask('task'), () => api.triggerTask('task'),
    () => api.suspendTask('task'), () => api.resumeTask('task'), () => api.getStats(),
    () => api.getTrend(), () => api.listWorkflows(), () => api.getWorkflow('workflow'),
    () => api.createWorkflow({ metadata: {}, spec: { tasks: [] } })
  ]
  for (const request of requests) {
    await request()
    const { url, options } = calls.at(-1)
    assert.equal(options.headers.Authorization, 'Bearer opaque-token')
    assert.equal(options.redirect, 'error')
    assert.equal(options.cache, 'no-store')
    assert.ok(!url.includes('opaque-token'))
    assert.ok(!options.body?.includes('opaque-token'))
  }
  setAuthToken('replacement')
  await api.listTasks()
  assert.equal(calls.at(-1).options.headers.Authorization, 'Bearer replacement')
  setAuthToken('')
  assert.equal(hasAuthToken.value, false)
  await api.listTasks()
  assert.equal(calls.at(-1).options.headers.Authorization, undefined)
})

test('invalid pasted values are rejected without changing the existing credential', () => {
  setAuthToken('valid')
  const revision = authRevision.value
  for (const value of ['Bearer valid', 'bad\r\nvalue', 'bad value', '===']) {
    assert.throws(() => setAuthToken(value), /原始 Token/)
  }
  assert.equal(authRevision.value, revision)
  assert.equal(hasAuthToken.value, true)
})

test('401 requests Token entry without exposing server text or retrying a mutation', async () => {
  const fetch = mock.method(globalThis, 'fetch', async () => json({ error: 'secret-from-upstream' }, 401))
  await assert.rejects(api.createTask({}), error => error.status === 401 && !error.message.includes('secret-from-upstream'))
  assert.equal(authRequired.value, true)
  assert.equal(fetch.mock.callCount(), 1)
  setAuthToken('new-token')
  assert.equal(authRequired.value, false)
})

test('403 and 503 keep the Token and do not request reauthentication', async () => {
  setAuthToken('valid')
  for (const status of [403, 503]) {
    mock.method(globalThis, 'fetch', async () => json({ error: status === 503 ? 'Service Unavailable' : 'Forbidden' }, status))
    await assert.rejects(api.listTasks(), error => error.status === status && /权限|稍后重试/.test(error.message))
    assert.equal(authRequired.value, false)
    assert.equal(hasAuthToken.value, true)
  }
})

test('changing Token aborts old requests and ignores their late 401 response', async () => {
  let finish
  let signal
  mock.method(globalThis, 'fetch', (_url, options) => {
    signal = options.signal
    return new Promise(resolve => { finish = resolve })
  })
  setAuthToken('old')
  const pending = api.listTasks()
  setAuthToken('new')
  assert.equal(signal.aborted, true)
  finish(json({ error: 'Unauthorized' }, 401))
  await assert.rejects(pending, { name: 'AbortError' })
  assert.equal(authRequired.value, false)
})

test('SSE sends the header, decodes fragmented UTF-8/CRLF and preserves empty/multiline events', async () => {
  const bytes = new TextEncoder().encode(': comment\r\nevent: message\r\ndata: 中文\r\ndata: second\r\n\r\ndata:\n\nevent: ignored\ndata: hidden\n\ndata: last\r\rdata: incomplete')
  let options
  let url
  mock.method(globalThis, 'fetch', async (requestURL, requestOptions) => {
    url = requestURL
    options = requestOptions
    return sse(new ReadableStream({ start(controller) {
      for (const byte of bytes) controller.enqueue(Uint8Array.of(byte))
      controller.close()
    } }))
  })
  setAuthToken('log-token')
  const lines = []
  await streamTaskLogs('task/name', { tail: 123, follow: false, onMessage: line => lines.push(line) })
  assert.equal(options.headers.Authorization, 'Bearer log-token')
  assert.equal(options.redirect, 'error')
  assert.equal(url, '/api/v1/tasks/task%2Fname/logs?tail=123&follow=false')
  assert.deepEqual(lines, ['中文\nsecond', '', 'last'])
})

test('anonymous SSE works and normal EOF is successful for both follow modes', async () => {
  mock.method(globalThis, 'fetch', async (_url, options) => {
    assert.equal(options.headers.Authorization, undefined)
    return sse('data: finished\n\n')
  })
  for (const follow of [false, true]) {
    const lines = []
    await streamTaskLogs('task', { tail: 1, follow, onMessage: line => lines.push(line) })
    assert.deepEqual(lines, ['finished'])
  }
})

test('SSE displays first event before EOF and cancellation closes the reader', async () => {
  let received
  const first = new Promise(resolve => { received = resolve })
  let cancelled = false
  mock.method(globalThis, 'fetch', async () => sse(new ReadableStream({
    start(controller) { controller.enqueue(new TextEncoder().encode('data: first\n\n')) },
    cancel() { cancelled = true }
  })))
  const controller = new AbortController()
  const pending = streamTaskLogs('task', { tail: 1, follow: true, signal: controller.signal, onMessage: received })
  const rejection = assert.rejects(pending, { name: 'AbortError' })
  assert.equal(await first, 'first')
  controller.abort()
  await rejection
  assert.equal(cancelled, true)
})

test('clearing Token closes an active SSE connection', async () => {
  let received
  const first = new Promise(resolve => { received = resolve })
  let signal
  let cancelled = false
  mock.method(globalThis, 'fetch', async (_url, options) => {
    signal = options.signal
    return sse(new ReadableStream({
      start(controller) { controller.enqueue(new TextEncoder().encode('data: first\n\n')) },
      cancel() { cancelled = true }
    }))
  })
  setAuthToken('valid')
  const pending = streamTaskLogs('task', { tail: 1, follow: true, onMessage: received })
  const rejection = assert.rejects(pending, { name: 'AbortError' })
  await first
  setAuthToken('')
  await rejection
  assert.equal(signal.aborted, true)
  assert.equal(cancelled, true)
})

test('SSE reports HTTP failures before parsing and rejects non-SSE responses', async () => {
  for (const status of [401, 403, 404, 503]) {
    setAuthToken('valid')
    mock.method(globalThis, 'fetch', async () => json({ error: 'Service Unavailable' }, status))
    await assert.rejects(streamTaskLogs('task', { tail: 1, follow: true, onMessage: () => assert.fail('must not emit logs') }), error => error.status === status)
    assert.equal(authRequired.value, status === 401)
  }
  mock.method(globalThis, 'fetch', async () => json({}))
  await assert.rejects(streamTaskLogs('task', { tail: 1, follow: false, onMessage() {} }), /未返回 SSE/)
})
