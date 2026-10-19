import test from 'node:test'
import assert from 'node:assert/strict'
import { layoutWorkflow, NODE_WIDTH, NODE_HEIGHT } from '../src/workflow/layout.js'

function assertGeometry(layout) {
  for (const node of layout.nodes) {
    assert.ok(Number.isFinite(node.x) && Number.isFinite(node.y))
    assert.ok(node.x >= 0 && node.y >= 0)
    assert.ok(node.x + NODE_WIDTH <= layout.width + 0.01)
    assert.ok(node.y + NODE_HEIGHT <= layout.height + 0.01)
  }
  for (let i = 0; i < layout.nodes.length; i++) {
    for (const other of layout.nodes.slice(i + 1)) {
      const node = layout.nodes[i]
      assert.ok(Math.abs(node.x - other.x) >= NODE_WIDTH || Math.abs(node.y - other.y) >= NODE_HEIGHT, 'nodes must not overlap')
    }
  }
  for (const edge of layout.edges) {
    assert.ok(edge.points.split(' ').length >= 2)
    assert.ok(edge.points.split(/[ ,]/).every(value => Number.isFinite(Number(value))))
  }
}

test('empty graph has no canvas content', () => {
  assert.deepEqual(layoutWorkflow(), { nodes: [], edges: [], width: 0, height: 0 })
})

test('parallel branches and joins flow left to right without overlap', () => {
  const layout = layoutWorkflow({
    nodes: ['build', 'test', 'lint', 'deploy'].map(name => ({ name })),
    edges: [{ from: 'build', to: 'test' }, { from: 'build', to: 'lint' }, { from: 'test', to: 'deploy' }, { from: 'lint', to: 'deploy' }]
  })
  assertGeometry(layout)
  const byName = new Map(layout.nodes.map(node => [node.name, node]))
  for (const edge of layout.edges) assert.ok(byName.get(edge.to).x > byName.get(edge.from).x)
  assert.equal(layout.edges.length, 4)
})

test('status, failure branch and child Task data survive layout without mutating API data', () => {
  const node = Object.freeze({ name: 'notify', phase: 'Skipped', runOnFailure: true, taskName: 'ci-notify', message: 'all dependencies succeeded' })
  const graph = { nodes: [Object.freeze({ name: 'test', phase: 'Failed' }), node], edges: [{ from: 'test', to: 'notify' }] }
  const before = JSON.stringify(graph)
  const layout = layoutWorkflow(graph)
  assert.equal(JSON.stringify(graph), before)
  assert.equal(layout.nodes.find(n => n.name === 'notify').message, node.message)
  assert.equal(layout.nodes.find(n => n.name === 'notify').taskName, 'ci-notify')
  assert.equal(layout.nodes.find(n => n.name === 'notify').phase, 'Skipped')
  assert.equal(layout.edges[0].failureBranch, true)
})

test('invalid dependency still renders as a missing node, alongside status-only nodes', () => {
  const layout = layoutWorkflow({ nodes: [{ name: 'test' }, { name: 'old-node', phase: 'Succeeded' }], edges: [{ from: 'missing', to: 'test' }] })
  assertGeometry(layout)
  assert.equal(layout.nodes.find(node => node.name === 'missing').missing, true)
  assert.equal(layout.nodes.find(node => node.name === 'missing').phase, undefined)
  assert.equal(layout.nodes.find(node => node.name === 'old-node').phase, 'Succeeded')
})

test('cyclic invalid spec remains inspectable without dropping dependency edges', () => {
  const layout = layoutWorkflow({ nodes: [{ name: 'a' }, { name: 'b' }], edges: [{ from: 'a', to: 'b' }, { from: 'b', to: 'a' }] })
  assertGeometry(layout)
  assert.equal(layout.edges.length, 2)
})

test('50 independent nodes remain non-overlapping and bounded', () => {
  const layout = layoutWorkflow({ nodes: Array.from({ length: 50 }, (_, i) => ({ name: `node-${i}` })), edges: [] })
  assertGeometry(layout)
  assert.equal(layout.nodes.length, 50)
})
