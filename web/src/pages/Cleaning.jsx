import React, { useEffect, useState } from 'react'
import { api } from '../api'
import { Modal, ErrorBox, OkBox } from '../components/ui'

// 清洗厂：待清洗队列 + 检测登记
export default function Cleaning({ user }) {
  const [queue, setQueue] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [target, setTarget] = useState(null)
  const canOperate = ['cleaner', 'admin'].includes(user.role)

  const load = () => api('/cleaning/queue').then(setQueue).catch(e => setError(e.message))
  useEffect(() => { load() }, [])

  return (
    <div>
      <h2 className="mb">清洗检测</h2>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />
      <div className="card">
        <h3>待清洗检测队列（{queue.length}）</h3>
        <table className="tbl">
          <thead><tr>
            <th>桶编号</th><th>容量</th><th>批次</th><th>回收外观</th><th>残液</th><th>封盖</th><th>操作</th>
          </tr></thead>
          <tbody>
            {queue.map(k => (
              <tr key={k.id}>
                <td>{k.keg_no}</td><td>{k.capacity_liter}L</td><td>{k.batch_no || '-'}</td>
                <td>{k.appearance || '-'}</td><td>{k.residual || '-'}</td><td>{k.cap_status || '-'}</td>
                <td>{canOperate && <button className="btn small primary" onClick={() => setTarget(k)}>登记检测</button>}</td>
              </tr>
            ))}
            {queue.length === 0 && <tr><td colSpan={7} className="muted">队列为空</td></tr>}
          </tbody>
        </table>
      </div>
      {target && <RecordModal keg={target} onClose={() => setTarget(null)}
        onDone={(passed) => { setTarget(null); setOk(passed ? '检测合格：酒桶获得再灌装资格，已通知财务释放押金' : '检测不合格：已自动生成异常事件'); load() }} />}
    </div>
  )
}

function RecordModal({ keg, onClose, onDone }) {
  const [result, setResult] = useState('passed')
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const submit = async () => {
    try {
      await api('/cleaning', { method: 'POST', body: { keg_id: keg.id, result, check_note: note } })
      onDone(result === 'passed')
    } catch (e) { setError(e.message) }
  }
  return (
    <Modal title={`清洗检测登记 · ${keg.keg_no}`} onClose={onClose}>
      <ErrorBox error={error} />
      <div className="form-item"><label>检测结果</label>
        <select value={result} onChange={e => setResult(e.target.value)}>
          <option value="passed">合格（可再灌装）</option>
          <option value="failed">不合格</option>
        </select></div>
      <div className="form-item mt"><label>检测说明（内窥 / 残碱 / 异味 / 压力）</label>
        <textarea rows={3} value={note} onChange={e => setNote(e.target.value)}
          placeholder="如：内窥无残留，残碱合格，无异味" /></div>
      {result === 'failed' && <div className="alert error mt">不合格将自动生成异常事件，由仓库/品控/财务协同评估重新清洗或报废。</div>}
      <div className="actions"><button className="btn primary" onClick={submit}>提交检测结果</button></div>
    </Modal>
  )
}
