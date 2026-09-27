import React from 'react'

export function Modal({ title, onClose, children, width }) {
  return (
    <div className="modal-mask" onClick={onClose}>
      <div className="modal" style={width ? { width } : {}} onClick={e => e.stopPropagation()}>
        <div className="flex-between">
          <h3>{title}</h3>
          <button className="btn small" onClick={onClose}>关闭</button>
        </div>
        <div className="mt">{children}</div>
      </div>
    </div>
  )
}

export function StatusBadge({ text, color }) {
  return <span className={`badge ${color || ''}`}>{text}</span>
}

// 酒桶状态颜色
export function KegStatusBadge({ status, name }) {
  const colors = {
    filled: 'blue', in_transit: 'purple', at_distributor: 'purple',
    delivered: 'yellow', opened: 'green', empty: 'gray',
    pickup_scheduled: 'yellow', returned: 'blue', clean_passed: 'green',
    clean_failed: 'red', lost: 'red', scrapped: 'gray',
  }
  return <span className={`badge ${colors[status] || ''}`}>{name || status}</span>
}

export function BatchStatusBadge({ status }) {
  const map = { normal: ['正常', 'green'], suspended: ['已停售', 'yellow'], recalled: ['已召回', 'red'] }
  const [txt, color] = map[status] || [status, '']
  return <span className={`badge ${color}`}>{txt}</span>
}

export function IncidentStatusBadge({ status }) {
  const map = { open: ['待处理', 'red'], processing: ['协同处理中', 'yellow'], resolved: ['已办结', 'green'], closed: ['已关闭', 'gray'] }
  const [txt, color] = map[status] || [status, '']
  return <span className={`badge ${color}`}>{txt}</span>
}

export function ErrorBox({ error, onClose }) {
  if (!error) return null
  return <div className="alert error flex-between"><span>{error}</span>{onClose && <a onClick={onClose} style={{cursor:'pointer'}}>×</a>}</div>
}

export function OkBox({ msg }) {
  if (!msg) return null
  return <div className="alert ok">{msg}</div>
}
