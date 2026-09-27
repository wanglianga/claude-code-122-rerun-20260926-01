import React, { useEffect, useState } from 'react'
import { api, fmtTime } from '../api'

const TYPE_NAMES = {
  recall: '批次召回', stop_sale: '批次停售', incident: '异常事件',
  deposit: '押金', pickup: '回收预约', order: '配送订单',
}
const TYPE_COLORS = { recall: 'red', stop_sale: 'yellow', incident: 'purple', deposit: 'green', pickup: 'blue', order: 'blue' }

export default function Notifications({ user }) {
  const [list, setList] = useState([])
  const load = () => api('/notifications').then(setList).catch(() => {})
  useEffect(() => { load() }, [])

  const markRead = async id => {
    await api(`/notifications/${id}/read`, { method: 'POST' }).catch(() => {})
    load()
  }
  const markAll = async () => {
    for (const n of list.filter(x => !x.read)) {
      await api(`/notifications/${n.id}/read`, { method: 'POST' }).catch(() => {})
    }
    load()
  }

  return (
    <div>
      <div className="flex-between mb">
        <h2>通知中心</h2>
        {list.some(n => !n.read) && <button className="btn" onClick={markAll}>全部已读</button>}
      </div>
      <div className="card">
        {list.length === 0 && <p className="muted">暂无通知</p>}
        {list.map(n => (
          <div key={n.id} className="flex-between" style={{
            padding: '10px 0', borderBottom: '1px solid #f3f4f6',
            opacity: n.read ? 0.6 : 1 }}>
            <div>
              <span className={`badge ${TYPE_COLORS[n.type] || ''}`}>{TYPE_NAMES[n.type] || n.type}</span>
              <b style={{marginLeft:8}}>{n.title}</b>
              {n.content && <div className="muted" style={{marginTop:4}}>{n.content}</div>}
              <div className="muted" style={{marginTop:2}}>{fmtTime(n.created_at)}</div>
            </div>
            {!n.read && <button className="btn small" onClick={() => markRead(n.id)}>标为已读</button>}
          </div>
        ))}
      </div>
    </div>
  )
}
