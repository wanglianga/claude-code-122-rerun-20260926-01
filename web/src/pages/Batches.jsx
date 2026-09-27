import React, { useEffect, useState } from 'react'
import { api, fmtDate, fmtTime, KEG_STATUS } from '../api'
import { Modal, BatchStatusBadge, ErrorBox, OkBox } from '../components/ui'

export default function Batches({ user }) {
  const [list, setList] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [detail, setDetail] = useState(null)
  const [suspendFor, setSuspendFor] = useState(null)
  const [recallFor, setRecallFor] = useState(null)
  const canManage = ['warehouse', 'qc', 'admin'].includes(user.role)
  const canQC = ['qc', 'admin'].includes(user.role)

  const load = () => api('/batches').then(setList).catch(e => setError(e.message))
  useEffect(() => { load() }, [])

  const openDetail = async id => {
    try { setDetail(await api('/batches/' + id)) } catch (e) { setError(e.message) }
  }

  return (
    <div>
      <div className="flex-between mb">
        <h2>批次管理</h2>
        {canManage && <button className="btn primary" onClick={() => setShowCreate(true)}>新建批次</button>}
      </div>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />
      <div className="card">
        <table className="tbl">
          <thead><tr>
            <th>批次号</th><th>酒型</th><th>灌装日期</th><th>最佳饮用期</th><th>酒精度</th>
            <th>桶数</th><th>严重反馈</th><th>状态</th><th>操作</th>
          </tr></thead>
          <tbody>
            {list.map(b => (
              <tr key={b.id}>
                <td><a onClick={() => openDetail(b.id)} style={{cursor:'pointer'}}>{b.batch_no}</a></td>
                <td>{b.beer_style}</td>
                <td>{b.fill_date}</td>
                <td>{b.best_before}</td>
                <td>{b.abv}%</td>
                <td>{b.keg_count}</td>
                <td>{b.bad_feedback_count > 0 ? <span className="badge red">{b.bad_feedback_count}</span> : 0}</td>
                <td><BatchStatusBadge status={b.status} /></td>
                <td className="flex">
                  <button className="btn small" onClick={() => openDetail(b.id)}>详情</button>
                  {canQC && b.status === 'normal' && <>
                    <button className="btn small warn" onClick={() => setSuspendFor(b)}>停售</button>
                    <button className="btn small danger" onClick={() => setRecallFor(b)}>召回</button>
                  </>}
                  {canQC && b.status === 'suspended' &&
                    <button className="btn small" onClick={async () => {
                      try { await api(`/batches/${b.id}/resume`, { method: 'POST' }); setOk('已恢复销售'); load() }
                      catch (e) { setError(e.message) }
                    }}>恢复销售</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {showCreate && <CreateBatchModal onClose={() => setShowCreate(false)} onDone={() => { setShowCreate(false); setOk('批次已创建'); load() }} />}
      {detail && <BatchDetailModal batch={detail} onClose={() => setDetail(null)} />}
      {suspendFor && <SuspendModal batch={suspendFor} onClose={() => setSuspendFor(null)}
        onDone={() => { setSuspendFor(null); setOk('批次已停售，已通知酒吧/司机/经销商'); load() }} />}
      {recallFor && <RecallModal batch={recallFor} onClose={() => setRecallFor(null)}
        onDone={(scope) => { setRecallFor(null); setOk('召回已发起：' + JSON.stringify(scope)); load() }} />}
    </div>
  )
}

function CreateBatchModal({ onClose, onDone }) {
  const [f, setF] = useState({ batch_no: '', beer_style: 'IPA', brew_date: '', fill_date: '', best_before: '', abv: 6.0 })
  const [error, setError] = useState('')
  const submit = async () => {
    try { await api('/batches', { method: 'POST', body: { ...f, abv: Number(f.abv) } }); onDone() }
    catch (e) { setError(e.message) }
  }
  return (
    <Modal title="新建酒液批次" onClose={onClose}>
      <ErrorBox error={error} />
      <div className="form-row">
        <div className="form-item"><label>批次号</label><input value={f.batch_no} onChange={e => setF({...f, batch_no: e.target.value})} placeholder="如 IPA-20261001" /></div>
        <div className="form-item"><label>酒型</label>
          <select value={f.beer_style} onChange={e => setF({...f, beer_style: e.target.value})}>
            {['IPA','世涛','小麦','酸啤'].map(s => <option key={s}>{s}</option>)}
          </select></div>
        <div className="form-item"><label>酒精度 %</label><input type="number" step="0.1" value={f.abv} onChange={e => setF({...f, abv: e.target.value})} /></div>
      </div>
      <div className="form-row">
        <div className="form-item"><label>酿造日期</label><input type="date" value={f.brew_date} onChange={e => setF({...f, brew_date: e.target.value})} /></div>
        <div className="form-item"><label>灌装日期</label><input type="date" value={f.fill_date} onChange={e => setF({...f, fill_date: e.target.value})} /></div>
        <div className="form-item"><label>最佳饮用期</label><input type="date" value={f.best_before} onChange={e => setF({...f, best_before: e.target.value})} /></div>
      </div>
      <div className="actions"><button className="btn primary" onClick={submit}>创建</button></div>
    </Modal>
  )
}

function BatchDetailModal({ batch: b, onClose }) {
  return (
    <Modal title={`批次 ${b.batch_no}（${b.beer_style}）`} onClose={onClose} width={720}>
      <div className="flex mb" style={{flexWrap:'wrap'}}>
        <BatchStatusBadge status={b.status} />
        <span>灌装 {b.fill_date}</span><span>最佳饮用期 {b.best_before}</span><span>酒精度 {b.abv}%</span>
        {b.stop_reason && <span className="badge red">{b.stop_reason}</span>}
      </div>
      <h4 className="mb">酒桶状态分布</h4>
      <div className="flex mb" style={{flexWrap:'wrap'}}>
        {Object.entries(b.keg_states || {}).map(([s, n]) => (
          <span key={s} className="badge blue">{KEG_STATUS[s] || s}：{n}</span>
        ))}
        {Object.keys(b.keg_states || {}).length === 0 && <span className="muted">暂无酒桶</span>}
      </div>
      <h4 className="mb">开桶反馈（{b.feedbacks.length}）</h4>
      {b.feedbacks.length === 0 && <p className="muted">暂无反馈</p>}
      {b.feedbacks.length > 0 && (
        <table className="tbl">
          <thead><tr><th>酒桶</th><th>酒吧</th><th>出酒</th><th>气压</th><th>口味异常</th><th>客诉</th><th>时间</th></tr></thead>
          <tbody>
            {b.feedbacks.map(f => (
              <tr key={f.id}>
                <td>{f.keg_no}</td><td>{f.bar_name}</td><td>{f.pour_speed}</td><td>{f.pressure_issue}</td>
                <td>{f.taste_anomaly !== '无' ? <span className="badge red">{f.taste_anomaly}</span> : '无'}{f.taste_note && <div className="muted">{f.taste_note}</div>}</td>
                <td>{f.complaint ? <span className="badge red">客诉</span> : '-'}{f.complaint_note && <div className="muted">{f.complaint_note}</div>}</td>
                <td>{fmtTime(f.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Modal>
  )
}

function SuspendModal({ batch, onClose, onDone }) {
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const submit = async () => {
    try { await api(`/batches/${batch.id}/suspend`, { method: 'POST', body: { reason } }); onDone() }
    catch (e) { setError(e.message) }
  }
  return (
    <Modal title={`停售批次 ${batch.batch_no}`} onClose={onClose}>
      <ErrorBox error={error} />
      <div className="alert info">停售后：已收货酒吧将收到「禁止开桶」通知，待配送订单暂停发货，经销商冻结库存。</div>
      <div className="form-item"><label>停售原因</label>
        <textarea rows={3} value={reason} onChange={e => setReason(e.target.value)} placeholder="如：多家酒吧反馈口味异常，品控评估暂停销售" /></div>
      <div className="actions"><button className="btn warn" onClick={submit}>确认停售</button></div>
    </Modal>
  )
}

function RecallModal({ batch, onClose, onDone }) {
  const [reason, setReason] = useState('')
  const [level, setLevel] = useState('全面召回')
  const [error, setError] = useState('')
  const submit = async () => {
    try {
      const d = await api('/recalls', { method: 'POST', body: { batch_id: batch.id, reason, level } })
      onDone(d.scope)
    } catch (e) { setError(e.message) }
  }
  return (
    <Modal title={`召回批次 ${batch.batch_no}`} onClose={onClose}>
      <ErrorBox error={error} />
      <div className="alert info">召回将自动圈定范围：已售卖 / 已开桶 / 未开桶 / 在库 / 在途 / 经销商库存，并通知相关方。</div>
      <div className="form-item"><label>召回级别</label>
        <select value={level} onChange={e => setLevel(e.target.value)}>
          <option>停售观察</option><option>全面召回</option>
        </select></div>
      <div className="form-item mt"><label>召回原因</label>
        <textarea rows={3} value={reason} onChange={e => setReason(e.target.value)} placeholder="如：批次酸败味投诉集中，疑似染菌" /></div>
      <div className="actions"><button className="btn danger" onClick={submit}>发起召回</button></div>
    </Modal>
  )
}
