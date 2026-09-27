import React, { useEffect, useState } from 'react'
import { api, fmtTime, INCIDENT_TYPES, ROLE_NAMES } from '../api'
import { Modal, IncidentStatusBadge, ErrorBox, OkBox } from '../components/ui'

// 异常协同：多方在同一事件内处理
export default function Incidents({ user }) {
  const [list, setList] = useState([])
  const [filter, setFilter] = useState('')
  const [error, setError] = useState('')
  const [detail, setDetail] = useState(null)

  const load = () => api('/incidents' + (filter ? `?status=${filter}` : '')).then(setList).catch(e => setError(e.message))
  useEffect(() => { load() }, [filter])

  return (
    <div>
      <div className="flex-between mb">
        <h2>异常协同处理</h2>
        <select value={filter} onChange={e => setFilter(e.target.value)}>
          <option value="">全部状态</option>
          <option value="open">待处理</option>
          <option value="processing">协同处理中</option>
          <option value="resolved">已办结</option>
        </select>
      </div>
      <ErrorBox error={error} onClose={() => setError('')} />
      <div className="card">
        <table className="tbl">
          <thead><tr>
            <th>事件号</th><th>类型</th><th>标题</th><th>酒桶</th><th>批次</th><th>酒吧</th><th>状态</th><th>协同记录</th><th>时间</th><th></th>
          </tr></thead>
          <tbody>
            {list.map(i => (
              <tr key={i.id}>
                <td>{i.incident_no}</td>
                <td><span className="badge purple">{INCIDENT_TYPES[i.type] || i.type}</span></td>
                <td>{i.title}</td>
                <td>{i.keg_no || '-'}</td><td>{i.batch_no || '-'}</td><td>{i.bar_name || '-'}</td>
                <td><IncidentStatusBadge status={i.status} /></td>
                <td>{i.action_count} 条</td>
                <td>{fmtTime(i.created_at)}</td>
                <td><button className="btn small" onClick={async () => setDetail(await api('/incidents/' + i.id))}>处理</button></td>
              </tr>
            ))}
            {list.length === 0 && <tr><td colSpan={10} className="muted">暂无异常事件</td></tr>}
          </tbody>
        </table>
      </div>
      {detail && <IncidentModal incident={detail} user={user} onClose={() => setDetail(null)}
        onChanged={async () => { setDetail(await api('/incidents/' + detail.id)); load() }} />}
    </div>
  )
}

function IncidentModal({ incident: i, user, onClose, onChanged }) {
  const [action, setAction] = useState('')
  const [note, setNote] = useState('')
  const [resolution, setResolution] = useState('')
  const [error, setError] = useState('')
  const canResolve = ['admin', 'qc', 'finance', 'warehouse'].includes(user.role)
  const open = ['open', 'processing'].includes(i.status)

  const addAction = async () => {
    if (!action.trim()) { setError('请填写处理动作'); return }
    try {
      await api(`/incidents/${i.id}/actions`, { method: 'POST', body: { action, note } })
      setAction(''); setNote(''); onChanged()
    } catch (e) { setError(e.message) }
  }
  const resolve = async () => {
    if (!resolution.trim()) { setError('请填写处理结论'); return }
    try {
      await api(`/incidents/${i.id}/resolve`, { method: 'POST', body: { resolution } })
      onChanged()
    } catch (e) { setError(e.message) }
  }

  return (
    <Modal title={`${i.incident_no} · ${INCIDENT_TYPES[i.type]}`} onClose={onClose} width={720}>
      <div className="flex mb" style={{flexWrap:'wrap'}}>
        <IncidentStatusBadge status={i.status} />
        {i.keg_no && <span className="badge blue">酒桶 {i.keg_no}</span>}
        {i.batch_no && <span className="badge purple">批次 {i.batch_no}</span>}
        {i.bar_name && <span className="badge yellow">{i.bar_name}</span>}
        <span className="muted">{fmtTime(i.created_at)}</span>
      </div>
      <h4 className="mb">{i.title}</h4>
      <p className="mb">{i.description}</p>
      {i.resolution && <div className="alert ok">处理结论：{i.resolution}</div>}

      <h4 className="mb">协同处理记录（仓库 / 酒吧 / 司机 / 清洗厂 / 品控 / 财务）</h4>
      <ul className="timeline">
        {(i.actions || []).map(a => (
          <li key={a.id}>
            <div className="t-time">{fmtTime(a.created_at)}</div>
            <div className="t-event">{a.action}
              <span className="badge blue" style={{marginLeft:8}}>{ROLE_NAMES[a.role] || a.role} · {a.user_name}</span></div>
            {a.note && <div className="t-detail">{a.note}</div>}
          </li>
        ))}
        {(i.actions || []).length === 0 && <p className="muted">暂无处理记录，等待各方响应</p>}
      </ul>

      {open && (
        <>
          <ErrorBox error={error} />
          <div className="form-row mt">
            <div className="form-item" style={{flex:2}}><label>我的处理动作</label>
              <input value={action} onChange={e => setAction(e.target.value)}
                placeholder={{warehouse:'如：核实库存/安排补扫',qc:'如：建议停售批次',finance:'如：押金扣款 500 元',cleaner:'如：重新清洗',driver:'如：补扫说明',bar_manager:'如：情况说明'}[user.role] || '处理动作'} /></div>
            <div className="form-item" style={{flex:2}}><label>说明</label>
              <input value={note} onChange={e => setNote(e.target.value)} placeholder="补充说明（可选）" /></div>
            <div className="form-item"><label>&nbsp;</label>
              <button className="btn primary" onClick={addAction}>提交处理</button></div>
          </div>
          {canResolve && (
            <div className="form-row" style={{borderTop:'1px solid #eef0f2', paddingTop:12}}>
              <div className="form-item" style={{flex:3}}><label>办结结论（押金扣款 / 再灌装资格 / 停售建议等）</label>
                <input value={resolution} onChange={e => setResolution(e.target.value)}
                  placeholder="如：扣除酒吧押金 500 元，酒桶报废，禁止再灌装" /></div>
              <div className="form-item"><label>&nbsp;</label>
                <button className="btn danger" onClick={resolve}>办结事件</button></div>
            </div>
          )}
        </>
      )}
    </Modal>
  )
}
