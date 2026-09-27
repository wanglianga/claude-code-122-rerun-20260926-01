// API 封装：自动携带 token，统一错误处理
const TOKEN_KEY = 'kegtrace_token'
const USER_KEY = 'kegtrace_user'

export function getToken() { return localStorage.getItem(TOKEN_KEY) || '' }
export function getUser() {
  try { return JSON.parse(localStorage.getItem(USER_KEY) || 'null') } catch { return null }
}
export function setAuth(token, user) {
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USER_KEY, JSON.stringify(user))
}
export function clearAuth() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

export async function api(path, opts = {}) {
  const res = await fetch('/api' + path, {
    method: opts.method || 'GET',
    headers: {
      'Content-Type': 'application/json',
      ...(getToken() ? { Authorization: 'Bearer ' + getToken() } : {}),
    },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  })
  if (res.status === 401) {
    clearAuth()
    if (!path.startsWith('/login')) window.location.href = '/login'
    throw new Error('未登录')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || `请求失败(${res.status})`)
  return data
}

export const ROLE_NAMES = {
  admin: '管理员', warehouse: '酒厂仓库', bar_manager: '酒吧店长', driver: '配送司机',
  cleaner: '清洗厂', qc: '品控', finance: '财务', distributor: '经销商',
}

export const KEG_STATUS = {
  filled: '在库已灌装', in_transit: '配送在途', at_distributor: '经销商库存',
  delivered: '酒吧未开桶', opened: '已开桶', empty: '已打空待回收',
  pickup_scheduled: '已预约回收', returned: '已回收待清洗',
  clean_passed: '清洗合格', clean_failed: '清洗不合格', lost: '丢失', scrapped: '报废',
}

export const INCIDENT_TYPES = {
  late_return: '延迟归还', lost: '酒桶丢失', deformed: '桶体变形',
  batch_complaint: '批次投诉', missed_scan: '司机漏扫', cleaning_failed: '清洗不合格',
}

export const RECALL_STATES = {
  sold: '已售卖', opened: '已开桶', unopened: '未开桶',
  in_warehouse: '酒厂在库', in_transit: '在途', at_distributor: '经销商库存',
}

export const DEPOSIT_TX_TYPES = { hold: '押金挂账', release: '押金释放', deduct: '押金扣款', adjust: '调整' }

export function fmtTime(t) {
  if (!t) return '-'
  return String(t).replace('T', ' ').slice(0, 16)
}
export function fmtDate(t) {
  if (!t) return '-'
  return String(t).slice(0, 10)
}
