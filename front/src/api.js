// 统一 fetch 封装：自动携带会话令牌（兼容无 Cookie 环境），解析错误。
function token() {
  try { return localStorage.getItem('vix_sid') || '' } catch (e) { return '' }
}

export async function api(path, opts = {}) {
  const headers = Object.assign({}, opts.headers || {})
  const t = token()
  if (t) headers['X-Vix-Sid'] = t
  let body = opts.body
  if (body && typeof body === 'object' && !(body instanceof FormData)) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(body)
  }
  const resp = await fetch(path, Object.assign({}, opts, { headers, body }))
  let data
  try { data = await resp.json() } catch (e) { data = null }
  if (resp.status === 401 && !path.startsWith('/login')) {
    // 会话失效：交给路由守卫跳转登录页。
    window.dispatchEvent(new CustomEvent('vix-unauthorized'))
  }
  if (!resp.ok) {
    const err = new Error((data && data.error) || ('HTTP ' + resp.status))
    err.status = resp.status
    err.data = data
    throw err
  }
  return data
}

export function saveToken(t) {
  try { t ? localStorage.setItem('vix_sid', t) : localStorage.removeItem('vix_sid') } catch (e) {}
}
