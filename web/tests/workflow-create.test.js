import test from 'node:test'
import assert from 'node:assert/strict'
import { api } from '../src/api/index.js'
import { setAuthToken } from '../src/api/auth.js'

test('workflow creation preserves nodes/templates and returns the generated name', async (t) => {
  setAuthToken('workflow-test-token')
  t.after(() => setAuthToken(''))
  const body = { metadata: {}, spec: { maxParallel: 2, tasks: [{ name: 'prepare', template: 'shell' }, { name: 'notify', dependsOn: ['prepare'], runOnFailure: true, taskSpec: { type: 'OneTime', image: 'busybox', command: ['sh', '-c', 'echo failure'], backoffLimit: 0 } }], taskTemplates: { shell: { type: 'OneTime', image: 'busybox', env: [{ name: 'MODE', value: 'qa' }] } } } }
  let count = 0
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    count++
    assert.equal(url, '/api/v1/workflows')
    assert.equal(options.method, 'POST')
    assert.equal(options.headers.Authorization, 'Bearer workflow-test-token')
    assert.deepEqual(JSON.parse(options.body), body)
    return new Response(JSON.stringify({ ...body, metadata: { name: 'workflow-generated' } }), { status: 201 })
  })
  const result = await api.createWorkflow(body)
  assert.equal(result.metadata.name, 'workflow-generated')
  assert.equal(count, 1)
})

test('workflow validation and name conflicts are surfaced without retrying creation', async (t) => {
  setAuthToken('')
  let count = 0
  const replies = [{ status: 400, error: 'unknown dependency' }, { status: 409, error: 'workflow already exists' }]
  t.mock.method(globalThis, 'fetch', async () => {
    const response = replies[count++]
    return new Response(JSON.stringify({ error: response.error }), { status: response.status })
  })
  await assert.rejects(api.createWorkflow({}), error => error.status === 400 && error.message === 'unknown dependency')
  await assert.rejects(api.createWorkflow({}), error => error.status === 409 && error.message === 'workflow already exists')
  assert.equal(count, 2)
})
