import React, { useEffect, useState } from 'react'
import { api, fmtTime, KEG_STATUS } from '../api'
import { Modal, KegStatusBadge, BatchStatusBadge, ErrorBox, OkBox } from '../components/ui'

export default function Kegs({ user }) {
  const [list, setList] = useState([])
  const [batches, setBatches] = useState([])
  const [filters, setFilters] = useState({ status: '', batch_id: '', q: '' })
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [detail, setDetail] = useState(null)
  const [refillFor, setRefillFor] = useState(null)
  const isWarehouse = ['warehouse', 'admin'].includes(user.role)

  const load = () => {
    const qs = new URLSearchParams(Object.entries(filters).filter(([,v]) => v)).toString()
    api('/kegs' + (qs ? '?' + qs : '')).then(setList).catch(e => setError(e.message))
  }
  useEffect(() => { load() }, [filters.status, filters.batch_id])
  useEffect(() => { api('/batches').then(setBatches).catch(() => {}) }, [])

  const openDetail = async id => {
    try { setDetail(await api('/kegs/' + id)) } catch (e) { setError(e.message) }
  }
  const reportLost = async k => {
    if (!confirm(`确认将酒桶 ${k.keg_no} 登记为丢失？将自动生成异常事件。`)) return
    try { await api(`/kegs/${k.id}/report-lost`, { method: 'POST' }); setOk('已登记丢失并生成异常事件'); load() }
    catch (e) { setError(e.message) }
  }

  return (
    <div>
      <div className="flex-between mb">
        <h2>酒桶追溯</h2>
        {isWarehouse && <button className="btn primary" onClick={() => setShowCreate(true)}>新桶灌装入库</button>}
      </div>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />
      <div className="card">
        <div className="form-row">
          <div className="form-item"><label>状态</label>
            <select value={filters.status} onChange={e => setFilters({...filters, status: e.target.value})}>
              <option value="">全部</option>
              {Object.entries(KEG_STATUS).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </select></div>
          <div className="form-item"><label>批次</label>
            <select value={filters.batch_id} onChange={e => setFilters({...filters, batch_id: e.target.value})}>
              <option value="">全部</option>
              {batches.map(b => <option key={b.id} value={b.id}>{b.batch_no}（{b.beer_style}）</option>)}
            </select></div>
          <div className="form-item"><label>桶编号</label>
            <input value={filters.q} onChange={e => setFilters({...filters, q: e.target.value})}
              onKeyDown={e => e.key === 'Enter' && load()} placeholder="回车搜索" /></div>
          <div className="form-item"><label>&nbsp;</label><button className="btn" onClick={load}>查询</button></div>
        </div>
        <table className="tbl">
          <thead><tr>
            <th>桶编号</th><th>容量</th><th>批次</th><th>押金</th><th>状态</th><th>当前位置</th><th>送达时间</th><th>操作</th>
          </tr></thead>
          <tbody>
            {list.map(k => (
              <tr key={k.id}>
                <td><a onClick={() => openDetail(k.id)} style={{cursor:'pointer'}}>{k.keg_no}</a></td>
                <td>{k.capacity_liter}L</td>
                <td>{k.batch_no || '-'}{k.beer_style && <span className="muted">（{k.beer_style}）</span>}</td>
                <td>¥{k.deposit_amount}</td>
                <td><KegStatusBadge status={k.status} name={k.status_name} /></td>
                <td>{k.location_name}</td>
                <td>{k.delivered_at ? fmtTime(k.delivered_at) : '-'}</td>
                <td className="flex">
                  <button className="btn small" onClick={() => openDetail(k.id)}>追溯</button>
                  {isWarehouse && k.status === 'clean_passed' &&
                    <button className="btn small primary" onClick={() => setRefillFor(k)}>再灌装</button>}
                  {isWarehouse && !['lost','scrapped'].includes(k.status) &&
                    <button className="btn small danger" onClick={() => reportLost(k)}>报失</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {list.length === 0 && <p className="muted mt">无匹配酒桶</p>}
      </div>

      {showCreate && <CreateKegModal batches={batches} onClose={() => setShowCreate(false)}
        onDone={() => { setShowCreate(false); setOk('酒桶已灌装入库'); load() }} />}
      {detail && <KegTraceModal keg={detail} onClose={() => setDetail(null)} />}
      {refillFor && <RefillModal keg={refillFor} batches={batches} onClose={() => setRefillFor(null)}
        onDone={() => { setRefillFor(null); setOk('再灌装完成'); load() }} />}
    </div>
  )
}

function CreateKegModal({ batches, onClose, onDone }) {
  const usable = batches.filter(b => b.status === 'normal')
  const [f, setF] = useState({ keg_no: '', capacity_liter: 30, batch_id: usable[0]?.id || '', deposit_amount: 500 })
  const [error, setError] = useState('')
  const submit = async () => {
    try {
      await api('/kegs', { method: 'POST', body: {
        keg_no: f.keg_no, capacity_liter: Number(f.capacity_liter),
        batch_id: Number(f.batch_id), deposit_amount: Number(f.deposit_amount) } })
      onDone()
    } catch (e) { setError(e.message) }
  }
  return (
    <Modal title="新桶灌装入库" onClose={onClose}>
      <ErrorBox error={error} />
      <div className="form-row">
        <div className="form-item"><label>桶编号（扫码）</label><input value={f.keg_no} onChange={e => setF({...f, keg_no: e.target.value})} placeholder="如 KEG-IPA-013" /></div>
        <div className="form-item"><label>容量（升）</label>
          <select value={f.capacity_liter} onChange={e => setF({...f, capacity_liter: e.target.value})}>
            <option value={20}>20L</option><option value={30}>30L</option><option value={50}>50L</option>
          </select></div>
      </div>
      <div className="form-row">
        <div className="form-item"><label>酒液批次</label>
          <select value={f.batch_id} onChange={e => setF({...f, batch_id: e.target.value})}>
            {usable.map(b => <option key={b.id} value={b.id}>{b.batch_no}（{b.beer_style}）</option>)}
          </select></div>
        <div className="form-item"><label>押金（元）</label><input type="number" value={f.deposit_amount} onChange={e => setF({...f, deposit_amount: e.target.value})} /></div>
      </div>
      <div className="actions"><button className="btn primary" onClick={submit}>入库</button></div>
    </Modal>
  )
}

function RefillModal({ keg, batches, onClose, onDone }) {
  const usable = batches.filter(b => b.status === 'normal')
  const [batchId, setBatchId] = useState(usable[0]?.id || '')
  const [error, setError] = useState('')
  const submit = async () => {
    try { await api(`/kegs/${keg.id}/refill`, { method: 'POST', body: { batch_id: Number(batchId) } }); onDone() }
    catch (e) { setError(e.message) }
  }
  return (
    <Modal title={`酒桶 ${keg.keg_no} 再灌装`} onClose={onClose}>
      <ErrorBox error={error} />
      <div className="alert info">该桶已清洗合格，具备再灌装资格。选择批次后重新入可发库存。</div>
      <div className="form-item"><label>目标批次</label>
        <select value={batchId} onChange={e => setBatchId(e.target.value)}>
          {usable.map(b => <option key={b.id} value={b.id}>{b.batch_no}（{b.beer_style}）</option>)}
        </select></div>
      <div className="actions"><button className="btn primary" onClick={submit}>确认再灌装</button></div>
    </Modal>
  )
}

// 单桶全链路追溯
export function KegTraceModal({ keg: k, onClose }) {
  return (
    <Modal title={`酒桶 ${k.keg_no} 全链路追溯`} onClose={onClose} width={760}>
      <div className="flex mb" style={{flexWrap:'wrap'}}>
        <KegStatusBadge status={k.status} name={k.status_name} />
        <span className="badge">{k.capacity_liter}L</span>
        <span className="badge yellow">押金 ¥{k.deposit_amount}</span>
        <span className="badge blue">{k.location_name}</span>
        {k.batch?.batch_no && <span className="badge purple">{k.batch.batch_no}（{k.batch.beer_style}）<BatchStatusBadge status={k.batch.status} /></span>}
      </div>
      {k.batch?.best_before && <p className="muted mb">最佳饮用期：{k.batch.best_before}；开桶时间：{fmtTime(k.opened_at)}；打空时间：{fmtTime(k.emptied_at)}</p>}
      <div className="grid2">
        <div>
          <h4 className="mb">生命周期时间线</h4>
          <ul className="timeline">
            {(k.events || []).map((e, i) => (
              <li key={i}>
                <div className="t-time">{fmtTime(e.created_at)}</div>
                <div className="t-event">{e.event}</div>
                {e.detail && <div className="t-detail">{e.detail}</div>}
                <div className="t-actor">操作人：{e.actor}</div>
              </li>
            ))}
            {(k.events || []).length === 0 && <p className="muted">暂无事件</p>}
          </ul>
        </div>
        <div>
          <h4 className="mb">开桶反馈</h4>
          {(k.feedbacks || []).length === 0 && <p className="muted">无</p>}
          {(k.feedbacks || []).map((f, i) => (
            <div key={i} className="card" style={{padding:10}}>
              <div>{f.bar_name} · {fmtTime(f.created_at)}</div>
              <div className="muted">出酒 {f.pour_speed} / 气压 {f.pressure_issue} / 口味 {f.taste_anomaly}{f.complaint ? ' / 有客诉' : ''}</div>
            </div>
          ))}
          <h4 className="mb mt">清洗检测</h4>
          {(k.cleaning_records || []).length === 0 && <p className="muted">无</p>}
          {(k.cleaning_records || []).map((r, i) => (
            <div key={i} className="card" style={{padding:10}}>
              <span className={`badge ${r.result === 'passed' ? 'green' : 'red'}`}>{r.result === 'passed' ? '合格' : '不合格'}</span>
              <span className="muted"> {r.operator} · {fmtTime(r.created_at)}</span>
              {r.check_note && <div className="muted">{r.check_note}</div>}
            </div>
          ))}
          <h4 className="mb mt">押金流水</h4>
          {(k.deposit_transactions || []).length === 0 && <p className="muted">无</p>}
          {(k.deposit_transactions || []).map((t, i) => (
            <div key={i} className="muted" style={{marginBottom:4}}>
              {fmtTime(t.created_at)} · {{hold:'挂账',release:'释放',deduct:'扣款',adjust:'调整'}[t.type]||t.type} ¥{t.amount}
              {t.status === 'pending' && <span className="badge yellow" style={{marginLeft:6}}>待结算</span>}
              {t.reason && <div>{t.reason}</div>}
            </div>
          ))}
        </div>
      </div>
    </Modal>
  )
}
