import React, { useEffect, useState } from 'react'
import { api } from '../api'
import { Modal, ErrorBox, OkBox } from '../components/ui'

// 基础数据：酒吧 / 经销商 / 用户
export default function BaseData({ user }) {
  const [tab, setTab] = useState('bars')
  const [bars, setBars] = useState([])
  const [dists, setDists] = useState([])
  const [users, setUsers] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [showBar, setShowBar] = useState(false)
  const [showDist, setShowDist] = useState(false)
  const isAdmin = user.role === 'admin'

  const load = () => {
    api('/bars').then(setBars).catch(e => setError(e.message))
    api('/distributors').then(setDists).catch(e => setError(e.message))
    if (isAdmin) api('/users').then(setUsers).catch(() => {})
  }
  useEffect(() => { load() }, [])

  return (
    <div>
      <h2 className="mb">基础数据</h2>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />
      <div className="tabs">
        <button className={tab === 'bars' ? 'active' : ''} onClick={() => setTab('bars')}>合作酒吧（{bars.length}）</button>
        <button className={tab === 'dists' ? 'active' : ''} onClick={() => setTab('dists')}>经销商（{dists.length}）</button>
        {isAdmin && <button className={tab === 'users' ? 'active' : ''} onClick={() => setTab('users')}>平台账号（{users.length}）</button>}
      </div>

      {tab === 'bars' && (
        <div className="card">
          <div className="flex-between mb">
            <h3>合作酒吧</h3>
            <button className="btn primary small" onClick={() => setShowBar(true)}>新增酒吧</button>
          </div>
          <table className="tbl">
            <thead><tr><th>名称</th><th>城市</th><th>地址</th><th>店长</th><th>所属经销商</th><th>押金账期</th></tr></thead>
            <tbody>
              {bars.map(b => (
                <tr key={b.id}>
                  <td>{b.name}</td><td>{b.city}</td><td>{b.address}</td><td>{b.manager_name}</td>
                  <td>{b.distributor_name || '酒厂直供'}</td><td>{b.deposit_credit_term_days} 天</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'dists' && (
        <div className="card">
          <div className="flex-between mb">
            <h3>多城市经销商</h3>
            <button className="btn primary small" onClick={() => setShowDist(true)}>新增经销商</button>
          </div>
          <table className="tbl">
            <thead><tr><th>名称</th><th>城市</th><th>联系人</th><th>电话</th><th>当前库存桶数</th></tr></thead>
            <tbody>
              {dists.map(d => (
                <tr key={d.id}>
                  <td>{d.name}</td><td>{d.city}</td><td>{d.contact}</td><td>{d.phone}</td>
                  <td>{d.stock_kegs}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'users' && isAdmin && (
        <div className="card">
          <h3>平台账号</h3>
          <table className="tbl">
            <thead><tr><th>用户名</th><th>姓名</th><th>角色</th><th>所属机构ID</th><th>电话</th></tr></thead>
            <tbody>
              {users.map(u => (
                <tr key={u.id}>
                  <td>{u.username}</td><td>{u.name}</td><td>{u.role}</td>
                  <td>{u.org_id || '-'}</td><td>{u.phone || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showBar && <BarModal dists={dists} onClose={() => setShowBar(false)}
        onDone={() => { setShowBar(false); setOk('酒吧已创建'); load() }} />}
      {showDist && <DistModal onClose={() => setShowDist(false)}
        onDone={() => { setShowDist(false); setOk('经销商已创建'); load() }} />}
    </div>
  )
}

function BarModal({ dists, onClose, onDone }) {
  const [f, setF] = useState({ name: '', city: '', address: '', manager_name: '', phone: '', distributor_id: '', deposit_credit_term_days: 30 })
  const [error, setError] = useState('')
  const submit = async () => {
    try {
      await api('/bars', { method: 'POST', body: {
        ...f, distributor_id: f.distributor_id ? Number(f.distributor_id) : null,
        deposit_credit_term_days: Number(f.deposit_credit_term_days) } })
      onDone()
    } catch (e) { setError(e.message) }
  }
  return (
    <Modal title="新增合作酒吧" onClose={onClose}>
      <ErrorBox error={error} />
      <div className="form-row">
        <div className="form-item"><label>名称</label><input value={f.name} onChange={e => setF({...f, name: e.target.value})} /></div>
        <div className="form-item"><label>城市</label><input value={f.city} onChange={e => setF({...f, city: e.target.value})} /></div>
      </div>
      <div className="form-row">
        <div className="form-item"><label>地址</label><input value={f.address} onChange={e => setF({...f, address: e.target.value})} /></div>
        <div className="form-item"><label>店长</label><input value={f.manager_name} onChange={e => setF({...f, manager_name: e.target.value})} /></div>
      </div>
      <div className="form-row">
        <div className="form-item"><label>所属经销商（空=酒厂直供）</label>
          <select value={f.distributor_id} onChange={e => setF({...f, distributor_id: e.target.value})}>
            <option value="">酒厂直供</option>
            {dists.map(d => <option key={d.id} value={d.id}>{d.name}（{d.city}）</option>)}
          </select></div>
        <div className="form-item"><label>押金账期（天）</label>
          <input type="number" value={f.deposit_credit_term_days} onChange={e => setF({...f, deposit_credit_term_days: e.target.value})} /></div>
      </div>
      <div className="actions"><button className="btn primary" onClick={submit}>创建</button></div>
    </Modal>
  )
}

function DistModal({ onClose, onDone }) {
  const [f, setF] = useState({ name: '', city: '', contact: '', phone: '' })
  const [error, setError] = useState('')
  const submit = async () => {
    try { await api('/distributors', { method: 'POST', body: f }); onDone() }
    catch (e) { setError(e.message) }
  }
  return (
    <Modal title="新增经销商" onClose={onClose}>
      <ErrorBox error={error} />
      <div className="form-row">
        <div className="form-item"><label>名称</label><input value={f.name} onChange={e => setF({...f, name: e.target.value})} /></div>
        <div className="form-item"><label>城市</label><input value={f.city} onChange={e => setF({...f, city: e.target.value})} /></div>
      </div>
      <div className="form-row">
        <div className="form-item"><label>联系人</label><input value={f.contact} onChange={e => setF({...f, contact: e.target.value})} /></div>
        <div className="form-item"><label>电话</label><input value={f.phone} onChange={e => setF({...f, phone: e.target.value})} /></div>
      </div>
      <div className="actions"><button className="btn primary" onClick={submit}>创建</button></div>
    </Modal>
  )
}
