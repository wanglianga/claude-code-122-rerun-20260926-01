import React, { useEffect, useState } from 'react'
import { api, fmtTime } from '../api'
import { Modal, ErrorBox, OkBox } from '../components/ui'

// 司机工作台：配送任务 + 回收扫码
export default function DriverHome({ user }) {
  const [tab, setTab] = useState('delivery')
  const [orders, setOrders] = useState([])
  const [appts, setAppts] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [deliverFor, setDeliverFor] = useState(null)
  const [scanAppt, setScanAppt] = useState(null)

  const load = () => {
    api('/orders').then(setOrders).catch(e => setError(e.message))
    api('/appointments').then(setAppts).catch(e => setError(e.message))
  }
  useEffect(() => { load() }, [])

  const act = async (path, msg) => {
    try { await api(path, { method: 'POST' }); setOk(msg); load() }
    catch (e) { setError(e.message) }
  }

  const myOrders = orders.filter(o => ['pending','delivering'].includes(o.status))
  const openAppts = appts.filter(a => ['scheduled','scanning'].includes(a.status))

  return (
    <div>
      <h2 className="mb">司机任务</h2>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />
      <div className="tabs">
        <button className={tab === 'delivery' ? 'active' : ''} onClick={() => setTab('delivery')}>配送任务（{myOrders.length}）</button>
        <button className={tab === 'pickup' ? 'active' : ''} onClick={() => setTab('pickup')}>空桶回收（{openAppts.length}）</button>
      </div>

      {tab === 'delivery' && (
        <div className="card">
          <table className="tbl">
            <thead><tr><th>单号</th><th>收货方</th><th>桶数</th><th>节日铺货</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              {myOrders.map(o => (
                <tr key={o.id}>
                  <td>{o.order_no}</td><td>{o.to_name}</td><td>{o.keg_count}</td>
                  <td>{o.is_holiday_temp ? <span className="badge purple">节日临时</span> : '-'}</td>
                  <td><span className={`badge ${o.status === 'pending' ? 'yellow' : 'purple'}`}>{o.status === 'pending' ? '待发货' : '配送中'}</span></td>
                  <td className="flex">
                    {o.status === 'pending' && <button className="btn small primary" onClick={() => act(`/orders/${o.id}/dispatch`, '已发货')}>发货</button>}
                    <button className="btn small primary" onClick={() => setDeliverFor(o)}>确认送达</button>
                  </td>
                </tr>
              ))}
              {myOrders.length === 0 && <tr><td colSpan={6} className="muted">暂无配送任务</td></tr>}
            </tbody>
          </table>
        </div>
      )}

      {tab === 'pickup' && (
        <div className="card">
          <table className="tbl">
            <thead><tr><th>预约单号</th><th>酒吧</th><th>预约日期</th><th>状态</th><th>已扫码</th><th>操作</th></tr></thead>
            <tbody>
              {openAppts.map(a => (
                <tr key={a.id}>
                  <td>{a.appt_no}</td><td>{a.bar_name}</td><td>{a.scheduled_date}</td>
                  <td><span className={`badge ${a.status === 'scheduled' ? 'yellow' : 'purple'}`}>{a.status === 'scheduled' ? '待接单' : '扫码中'}</span></td>
                  <td>{a.scan_count}</td>
                  <td className="flex">
                    {a.status === 'scheduled' && !a.driver_id &&
                      <button className="btn small primary" onClick={() => act(`/appointments/${a.id}/claim`, '已接单，请到店扫码')}>接单</button>}
                    {(a.status === 'scanning' || a.driver_id === user.id) &&
                      <button className="btn small primary" onClick={() => setScanAppt(a)}>扫码回收</button>}
                  </td>
                </tr>
              ))}
              {openAppts.length === 0 && <tr><td colSpan={6} className="muted">暂无回收任务</td></tr>}
            </tbody>
          </table>
        </div>
      )}

      {deliverFor && <DeliverModal order={deliverFor} onClose={() => setDeliverFor(null)}
        onDone={() => { setDeliverFor(null); setOk('已送达，押金挂账完成'); load() }} />}
      {scanAppt && <ScanModal appt={scanAppt} onClose={() => { setScanAppt(null); load() }} />}
    </div>
  )
}

function DeliverModal({ order, onClose, onDone }) {
  const [receiver, setReceiver] = useState('')
  const [error, setError] = useState('')
  const submit = async () => {
    try { await api(`/orders/${order.id}/deliver`, { method: 'POST', body: { receiver_name: receiver } }); onDone() }
    catch (e) { setError(e.message) }
  }
  return (
    <Modal title={`确认送达 ${order.order_no}（${order.to_name}）`} onClose={onClose}>
      <ErrorBox error={error} />
      <div className="form-item"><label>酒吧收货人</label>
        <input value={receiver} onChange={e => setReceiver(e.target.value)} placeholder="收货人姓名" /></div>
      <div className="actions"><button className="btn primary" onClick={submit}>确认送达</button></div>
    </Modal>
  )
}

// 回收扫码：逐桶扫码确认 数量/外观/残液/封盖
function ScanModal({ appt, onClose }) {
  const [detail, setDetail] = useState(null)
  const [kegNo, setKegNo] = useState('')
  const [appearance, setAppearance] = useState('完好')
  const [residual, setResidual] = useState('无残液')
  const [capStatus, setCapStatus] = useState('封盖完好')
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [finishing, setFinishing] = useState(false)

  const load = () => api('/appointments/' + appt.id).then(setDetail).catch(e => setError(e.message))
  useEffect(() => { load() }, [])

  const scan = async () => {
    setError(''); setOk('')
    try {
      await api(`/appointments/${appt.id}/scan`, { method: 'POST',
        body: { keg_no: kegNo.trim(), appearance, residual, cap_status: capStatus } })
      setOk(`已扫码 ${kegNo.trim()}`)
      setKegNo('')
      load()
    } catch (e) { setError(e.message) }
  }
  const finish = async () => {
    setFinishing(true)
    try {
      const d = await api(`/appointments/${appt.id}/complete`, { method: 'POST' })
      if (d.missed_count > 0) alert(`回收完成，但有 ${d.missed_count} 只空桶漏扫，平台已生成异常事件`)
      onClose()
    } catch (e) { setError(e.message); setFinishing(false) }
  }

  const pending = detail?.pending_kegs || []
  const scans = detail?.scans || []
  return (
    <Modal title={`回收扫码 · ${appt.appt_no}（${appt.bar_name}）`} onClose={onClose} width={680}>
      <ErrorBox error={error} />
      <OkBox msg={ok} />
      <div className="form-row" style={{alignItems:'flex-end'}}>
        <div className="form-item" style={{flex:1}}><label>扫描/输入桶编号</label>
          <input autoFocus value={kegNo} onChange={e => setKegNo(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && kegNo.trim() && scan()} placeholder="如 KEG-IPA-008" /></div>
        <div className="form-item"><label>外观</label>
          <select value={appearance} onChange={e => setAppearance(e.target.value)}>
            {['完好','轻微磕碰','桶体变形'].map(x => <option key={x}>{x}</option>)}
          </select></div>
        <div className="form-item"><label>残液</label>
          <select value={residual} onChange={e => setResidual(e.target.value)}>
            {['无残液','少量','较多'].map(x => <option key={x}>{x}</option>)}
          </select></div>
        <div className="form-item"><label>封盖</label>
          <select value={capStatus} onChange={e => setCapStatus(e.target.value)}>
            {['封盖完好','封盖缺失','封盖损坏'].map(x => <option key={x}>{x}</option>)}
          </select></div>
        <button className="btn primary" onClick={scan} disabled={!kegNo.trim()}>确认扫码</button>
      </div>
      <div className="grid2 mt">
        <div>
          <h4 className="mb">待回收（{pending.filter(k => !k.scanned).length}）</h4>
          {pending.filter(k => !k.scanned).map(k => (
            <div key={k.keg_id} className="flex-between" style={{padding:'6px 0', borderBottom:'1px solid #f3f4f6'}}>
              <span>{k.keg_no} <span className="muted">{k.batch_no}</span></span>
              <button className="btn small" onClick={() => setKegNo(k.keg_no)}>选中</button>
            </div>
          ))}
          {pending.filter(k => !k.scanned).length === 0 && <p className="muted">全部扫码完成</p>}
        </div>
        <div>
          <h4 className="mb">已扫码（{scans.length}）</h4>
          {scans.map(s => (
            <div key={s.keg_id} style={{padding:'6px 0', borderBottom:'1px solid #f3f4f6'}}>
              <b>{s.keg_no}</b>
              <div className="muted">{s.appearance} / {s.residual} / {s.cap_status}</div>
            </div>
          ))}
        </div>
      </div>
      <div className="alert info mt">完成回收时如有空桶未扫码，平台将自动生成「司机漏扫」异常事件。</div>
      <div className="actions">
        <button className="btn primary" onClick={finish} disabled={finishing}>{finishing ? '提交中…' : '完成回收'}</button>
      </div>
    </Modal>
  )
}
