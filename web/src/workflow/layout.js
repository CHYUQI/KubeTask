import dagre from '@dagrejs/dagre'

export const NODE_WIDTH = 210
export const NODE_HEIGHT = 84

// Only calculate display coordinates; dependency validity and execution order
// remain the responsibility of the API and Controller.
export function layoutWorkflow(graph = {}) {
  const nodes = new Map((graph.nodes || []).map(node => [node.name, { ...node }]))
  const edges = graph.edges || []
  for (const edge of edges) {
    for (const name of [edge.from, edge.to]) {
      if (!nodes.has(name)) nodes.set(name, { name, missing: true })
    }
  }
  if (!nodes.size) return { nodes: [], edges: [], width: 0, height: 0 }

  const model = new dagre.graphlib.Graph()
  model.setGraph({ rankdir: 'LR', nodesep: 36, ranksep: 70, marginx: 28, marginy: 28 })
  model.setDefaultEdgeLabel(() => ({}))
  for (const name of nodes.keys()) model.setNode(name, { width: NODE_WIDTH, height: NODE_HEIGHT })
  for (const edge of edges) model.setEdge(edge.from, edge.to)
  dagre.layout(model)

  return {
    nodes: [...nodes.values()].map(node => {
      const position = model.node(node.name)
      return { ...node, x: position.x - NODE_WIDTH / 2, y: position.y - NODE_HEIGHT / 2 }
    }),
    edges: model.edges().map((edge, index) => ({
      id: `edge-${index}`,
      from: edge.v,
      to: edge.w,
      failureBranch: Boolean(nodes.get(edge.w)?.runOnFailure),
      points: model.edge(edge).points.map(point => `${point.x},${point.y}`).join(' ')
    })),
    width: model.graph().width,
    height: model.graph().height
  }
}
