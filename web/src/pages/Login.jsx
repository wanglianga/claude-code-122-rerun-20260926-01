import React, { useState } from 'react'
import { api, setAuth } from '../api'

export default function Login({ onLogin }) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('123456')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async e => {
    e.preventDefault()
    setLoading(true); setError('')
    try {
      const d = await api('/login', { method: 'POST', body: { username, password } })
      setAuth(d.token, d.user)
      onLogin(d.user)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <form className="login-box" onSubmit={submit}>
        <h1>精酿空桶追溯平台</h1>
        <div className="sub">空桶回收押金与批次追溯协同系统</div>
        {error && <div className="alert error">{error}</div>}
        <input value={username} onChange={e => setUsername(e.target.value)} placeholder="用户名" />
        <input type="password" value={password} onChange={e => setPassword(e.target.value)} placeholder="密码" />
        <button className="btn primary" disabled={loading}>{loading ? '登录中…' : '登 录'}</button>
        <div className="hint">
          测试账号（密码均为 123456）：<br />
          admin 管理员 / warehouse 仓库 / qc 品控 / finance 财务<br />
          cleaner 清洗厂 / driver1、driver2 司机 / dist_hz、dist_sh 经销商<br />
          bar1~bar4 酒吧店长
        </div>
      </form>
    </div>
  )
}
