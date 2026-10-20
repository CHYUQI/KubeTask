export const API_BASE = '/api/v1'
export const REQUEST_TIMEOUT = 10000

export class APIError extends Error {
  constructor(message, status) {
    super(message)
    this.name = 'APIError'
    this.status = status
  }
}

export async function checkAPIResponse(response, request) {
  if (response.ok) return
  if (response.status === 401) {
    request.requireAuthentication()
    throw new APIError('需要有效的 ServiceAccount Token，请在页面上方设置或更换 Token', 401)
  }
  if (response.status === 403) throw new APIError('没有执行此操作的权限', 403)
  let message = response.statusText
  try { message = (await response.json()).error || message } catch { /* 非 JSON 错误保留状态描述。 */ }
  if (response.status === 503 && message === 'Service Unavailable') message = '服务暂时不可用，请稍后重试'
  throw new APIError(message || `请求失败（${response.status}）`, response.status)
}
