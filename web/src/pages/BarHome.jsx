import React, { useEffect, useState } from 'react'
import { api, fmtTime, fmtDate } from '../api'
import { Modal, KegStatusBadge, ErrorBox, OkBox } from '../components/ui'

// 酒吧店长工作台：我的酒桶 + 开桶反馈 + 回收预约
export default function BarHome({ user }) {
  const [kegs, setKegs] = useState([])
  const [appts, setAppts] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [feedbackFor, setFeedbackFor] = useState(null)
  const [showAppt, setShowAppt] = useState(false)

  const load = () => {
    if (!user.org_id) return
    api(`/kegs?bar_id=${user.org_id}`).then(setKegs).catch(e => setError(e.message))
    api('/appointments').then(setAppts).catch(() => {})
  }
  useEffect(() => { load() }, [])

  const act = async (path, okMsg) => {
    try { await api(path, { method: 'POST' }); setOk(okMsg); load() }
    catch (e) { setError(e.message) }
  }

  const active = kegs.filter(k => !['lost','scrapped'].includes(k.status))
  return (
    <div>
      <div className="flex-between mb">
        <h2>我的酒吧 {user.bar_name ? `· ${user.bar_name}` : ''}</h2>
        <button className="btn primary" onClick={() => setShowAppt(true)}>预约空桶回收</button>
      </div>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />

      <div className="card">
        <h3>店内酒桶（{active.length}）</h3>
        <table className="tbl">
          <thead><tr>
            <th>桶编号</th><th>批次</th><th>状态</th><th>送达时间</th><th>开桶时间</th><th>操作</th>
          </tr></thead>
          <tbody>
            {active.map(k => (
              <tr key={k.id}>
                <td>{k.keg_no}</td>
                <td>{k.batch_no}<span className="muted">（{k.beer_style}）</span></td>
                <td><KegStatusBadge status={k.status} name={k.status_name} /></td>
                <td>{fmtTime(k.delivered_at)}</td>
                <td>{fmtTime(k.opened_at)}</td>
                <td className="flex">
                  {k.status === 'delivered' && <>
                    <button className="btn small primary" onClick={() => act(`/kegs/${k.id}/open`, '已开桶')}>开桶</button>
                    <button className="btn small" onClick={() => setFeedbackFor(k)}>反馈</button>
                  </>}
                  {k.status === 'opened' && <>
                    <button className="btn small" onClick={() => setFeedbackFor(k)}>反馈</button>
                    <button className="btn small warn" onClick={() => { if (confirm('确认该桶已打空？')) act(`/kegs/${k.id}/empty`, '已确认打空，可预约回收') }}>确认打空</button>
                  </>}
                  {['empty','pickup_scheduled'].includes(k.status) && <span className="muted">待回收</span>}
                </td>
              </tr>
            ))}
            {active.length === 0 && <tr><td colSpan={6} className="muted">店内暂无酒桶</td></tr>}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h3>回收预约记录</h3>
        <table className="tbl">
          <thead><tr><th>预约单号</th><th>预约日期</th><th>状态</th><th>司机</th><th>已扫码</th><th>创建时间</th></tr></thead>
          <tbody>
            {appts.map(a => (
              <tr key={a.id}>
                <td>{a.appt_no}</td><td>{a.scheduled_date}</td>
                <td><span className={`badge ${{scheduled:'yellow',scanning:'purple',completed:'green',cancelled:'gray'}[a.status]}`}>
                  {{scheduled:'已预约',scanning:'扫码回收中',completed:'已完成',cancelled:'已取消'}[a.status]}</span></td>
                <td>{a.driver_name || '待接单'}</td><td>{a.scan_count}</td><td>{fmtTime(a.created_at)}</td>
              </tr>
            ))}
            {appts.length === 0 && <tr><td colSpan={6} className="muted">暂无预约</td></tr>}
          </tbody>
        </table>
      </div>

      {feedbackFor && <FeedbackModal keg={feedbackFor} onClose={() => setFeedbackFor(null)}
        onDone={() => { setFeedbackFor(null); setOk('反馈已提交，严重问题将自动通知品控') }} />}
      {showAppt && <ApptModal onClose={() => setShowAppt(false)}
        onDone={(n) => { setShowAppt(false); setOk(`回收预约已创建（${n} 只空桶），司机将收到通知`); load() }} />}
    </div>
  )
}

function FeedbackModal({ keg, onClose, onDone }) {
  const [f, setF] = useState({ pour_speed: '正常', pressure_issue: '正常', taste_anomaly: '无', taste_note: '', complaint: false, complaint_note: '' })
  const [error, setError] = useState('')
  const submit = async () => {
    try { await api('/feedbacks', { method: 'POST', body: { ...f, keg_id: keg.id } }); onDone() }
    catch (e) { setError(e.message) }
  }
  return (
    <Modal title={`开桶反馈 · ${keg.keg_no}（${keg.batch_no}）`} onClose={onClose}>
      <ErrorBox error={error} />
      <div className="form-row">
        <div className="form-item"><label>出酒速度</label>
          <select value={f.pour_speed} onChange={e => setF({...f, pour_speed: e.target.value})}>
            {['正常','偏慢','偏快','不出酒'].map(x => <option key={x}>{x}</option>)}
          </select></div>
        <div className="form-item"><label>气压</label>
          <select value={f.pressure_issue} onChange={e => setF({...f, pressure_issue: e.target.value})}>
            {['正常','气压不足','气压过高'].map(x => <option key={x}>{x}</option>)}
          </select></div>
        <div className="form-item"><label>口味异常</label>
          <select value={f.taste_anomaly} onChange={e => setF({...f, taste_anomaly: e.target.value})}>
            {['无','轻微','明显','严重'].map(x => <option key={x}>{x}</option>)}
          </select></div>
      </div>
      <div className="form-item"><label>口味描述</label>
        <input value={f.taste_note} onChange={e => setF({...f, taste_note: e.target.value})} placeholder="如：氧化纸板味、酸败味" /></div>
      <div className="form-item mt">
        <label style={{fontSize:13}}><input type="checkbox" checked={f.complaint} onChange={e => setF({...f, complaint: e.target.checked})} /> 有客诉</label>
      </div>
      {f.complaint && (
        <div className="form-item mt"><label>客诉说明</label>
          <textarea rows={2} value={f.complaint_note} onChange={e => setF({...f, complaint_note: e.target.value})} /></div>
      )}
      <div className="alert info mt">同一批次被多家酒吧严重反馈时，平台将自动生成批次投诉异常并建议停售。</div>
      <div className="actions"><button className="btn primary" onClick={submit}>提交反馈</button></div>
    </Modal>
  )
}

function ApptModal({ onClose, onDone }) {
  const [date, setDate] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const submit = async () => {
    try {
      const d = await api('/appointments', { method: 'POST', body: { scheduled_date: date, note } })
      onDone(d.keg_count)
    } catch (e) { setError(e.message) }
  }
  return (
    <Modal title="预约空桶回收" onClose={onClose}>
      <ErrorBox error={error} />
      <div className="alert info">预约后，店内所有「已打空」酒桶将标记为待回收，司机接单后上门扫码。</div>
      <div className="form-item"><label>预约日期</label>
        <input type="date" value={date} onChange={e => setDate(e.target.value)} /></div>
      <div className="form-item mt"><label>备注</label>
        <input value={note} onChange={e => setNote(e.target.value)} placeholder="如：下午 2 点后有人收货" /></div>
      <div className="actions"><button className="btn primary" onClick={submit}>提交预约</button></div>
    </Modal>
  )
}
