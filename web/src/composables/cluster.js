import { ref } from 'vue'

const CLUSTER_KEYWORDS = ['no kubernetes cluster', 'Bad Gateway', 'Failed to fetch', '请求超时']

export function useClusterStatus() {
  const clusterError = ref('')

  function isClusterError(err) {
    const msg = err?.message || err?.toString()
    return CLUSTER_KEYWORDS.some(k => msg.includes(k))
  }

  function handleApiError(err, fallback) {
    if (isClusterError(err)) {
      if (err.message?.includes('no kubernetes cluster')) {
        clusterError.value = err.message
      } else {
        clusterError.value = '无法连接到后端服务'
      }
      return null
    }
    return typeof fallback === 'function' ? fallback(err) : err.message
  }

  return { clusterError, isClusterError, handleApiError }
}
