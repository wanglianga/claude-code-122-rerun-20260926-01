import React, { useEffect, useState } from 'react'
import { Routes, Route, NavLink, Navigate, useNavigate } from 'react-router-dom'
import { api, getUser, clearAuth, ROLE_NAMES } from './api'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Batches from './pages/Batches'
import Kegs from './pages/Kegs'
import Orders from './pages/Orders'
import BarHome from './pages/BarHome'
import DriverHome from './pages/DriverHome'
import Cleaning from './pages/Cleaning'
import Deposits from './pages/Deposits'
import Incidents from './pages/Incidents'
import Recalls from './pages/Recalls'
import Notifications from './pages/Notifications'
import BaseData from './pages/BaseData'

// 角色可见菜单
const MENUS = [
  { path: '/', label: '工作台', roles: ['admin','warehouse','qc','finance','cleaner','driver','bar_manager','distributor'] },
  { path: '/bar', label: '我的酒吧', roles: ['bar_manager'] },
  { path: '/driver', label: '司机任务', roles: ['driver'] },
  { path: '/batches', label: '批次管理', roles: ['admin','warehouse','qc'] },
  { path: '/kegs', label: '酒桶追溯', roles: ['admin','warehouse','qc','finance','distributor'] },
  { path: '/orders', label: '配送订单', roles: ['admin','warehouse','distributor','bar_manager'] },
  { path: '/cleaning', label: '清洗检测', roles: ['admin','warehouse','cleaner'] },
  { path: '/deposits', label: '押金财务', roles: ['admin','finance','bar_manager','distributor'] },
  { path: '/incidents', label: '异常协同', roles: ['admin','warehouse','qc','finance','cleaner','driver','bar_manager','distributor'] },
  { path: '/recalls', label: '批次召回', roles: ['admin','warehouse','qc'] },
  { path: '/notifications', label: '通知中心', roles: ['admin','warehouse','qc','finance','cleaner','driver','bar_manager','distributor'] },
  { path: '/basedata', label: '基础数据', roles: ['admin','warehouse'] },
]

function Layout({ user, children }) {
  const nav = useNavigate()
  const [unread, setUnread] = useState(0)
  useEffect(() => {
    const load = () => api('/notifications/unread-count').then(d => setUnread(d.count)).catch(() => {})
    load()
    const t = setInterval(load, 15000)
    return () => clearInterval(t)
  }, [])
  const logout = async () => {
    try { await api('/logout', { method: 'POST' }) } catch {}
    clearAuth()
    nav('/login')
  }
  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="brand">
          精酿空桶追溯平台
          <small>押金 · 回收 · 批次协同</small>
        </div>
        <nav>
          {MENUS.filter(m => m.roles.includes(user.role)).map(m => (
            <NavLink key={m.path} to={m.path} end={m.path === '/'}
              className={({ isActive }) => isActive ? 'active' : ''}>
              {m.label}{m.path === '/notifications' && unread > 0 ? ` (${unread})` : ''}
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="main">
        <div className="topbar">
          <div className="muted">精酿酒吧空桶回收押金与批次追溯平台</div>
          <div className="who">
            <span className="badge blue">{ROLE_NAMES[user.role] || user.role}</span>
            <span>{user.name}{user.bar_name ? ` · ${user.bar_name}` : ''}{user.distributor_name ? ` · ${user.distributor_name}` : ''}</span>
            {unread > 0 && <span className="notif-badge">{unread} 条未读</span>}
            <button className="btn small" onClick={logout}>退出</button>
          </div>
        </div>
        <div className="content">{children}</div>
      </div>
    </div>
  )
}

export default function App() {
  const [user, setUser] = useState(getUser())
  // 登录后/刷新时拉取完整用户信息（含酒吧名、经销商名）
  useEffect(() => {
    if (user) {
      api('/me').then(u => setUser(prev => ({ ...prev, ...u }))).catch(() => {})
    }
  }, [user?.id])
  if (!user) {
    return (
      <Routes>
        <Route path="*" element={<Login onLogin={u => setUser(u)} />} />
      </Routes>
    )
  }
  return (
    <Layout user={user}>
      <Routes>
        <Route path="/" element={<Dashboard user={user} />} />
        <Route path="/bar" element={<BarHome user={user} />} />
        <Route path="/driver" element={<DriverHome user={user} />} />
        <Route path="/batches" element={<Batches user={user} />} />
        <Route path="/kegs" element={<Kegs user={user} />} />
        <Route path="/orders" element={<Orders user={user} />} />
        <Route path="/cleaning" element={<Cleaning user={user} />} />
        <Route path="/deposits" element={<Deposits user={user} />} />
        <Route path="/incidents" element={<Incidents user={user} />} />
        <Route path="/recalls" element={<Recalls user={user} />} />
        <Route path="/notifications" element={<Notifications user={user} />} />
        <Route path="/basedata" element={<BaseData user={user} />} />
        <Route path="*" element={<Navigate to="/" />} />
      </Routes>
    </Layout>
  )
}
