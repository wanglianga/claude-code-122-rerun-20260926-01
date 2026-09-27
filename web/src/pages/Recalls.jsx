import React, { useEffect, useState } from 'react'
import { api, fmtTime, RECALL_STATES } from '../api'
import { Modal, ErrorBox, OkBox } from '../components/ui'

// 批次召回：范围圈定与进度跟踪
export default function Recalls({ user }) {
  const [list, setList] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [detail, setDetail] = useState(null)
  const canOperate = ['warehouse', 'qc', 'admin'].includes(user.role)

  const load = () => api('/recalls').then(setList).catch(e => setError(e.message))
  useEffect(() => { load() }, [])

  return (
    <div>
      <div className="flex-between mb">
        <h2>批次召回</h2>
        <span className="muted">在「批次管理」中对批次发起召回</span>
      </div>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />
      <div className="card">
        <table className="tbl">
          <thead><tr>
            <th>召回单号</th><th>批次</th><th>级别</th><th>原因</th><th>状态</th><th>处理进度</th><th>发起时间</th><th></th>
          </tr></thead>
          <tbody>
            {list.map(r => (
              <tr key={r.id}>
                <td>{r.recall_no}</td>
                <td>{r.batch_no}（{r.beer_style}）</td>
                <td><span className={`badge ${r.level === '全面召回' ? 'red' : 'yellow'}`}>{r.level}</span></td>
                <td>{r.reason}</td>
                <td><span className={`badge ${r.status === 'active' ? 'yellow' : 'green'}`}>{r.status === 'active' ? '进行中' : '已完成'}</span></td>
                <td>{r.done_items}/{r.total_items}</td>
                <td>{fmtTime(r.created_at)}</td>
                <td><button className="btn small" onClick={async () => setDetail(await api('/recalls/' + r.id))}>详情</button></td>
              </tr>
            ))}
            {list.length === 0 && <tr><td colSpan={8} className="muted">暂无召回记录</td></tr>}
          </tbody>
        </table>
      </div>
      {detail && <RecallDetailModal recall={detail} canOperate={canOperate}
        onClose={() => setDetail(null)}
        onChanged={async () => { setDetail(await api('/recalls/' + detail.id)); load() }} />}
    </div>
  )
}

function RecallDetailModal({ recall: r, canOperate, onClose, onChanged }) {
  const [error, setError] = useState('')
  const update = async (itemId, status) => {
    const note = prompt('处理备注（可选）') || ''
    try {
      await api(`/recalls/${r.id}/items/${itemId}`, { method: 'POST', body: { status, note } })
      onChanged()
    } catch (e) { setError(e.message) }
  }
  const sc = r.state_count || {}
  return (
    <Modal title={`召回单 ${r.recall_no} · 批次 ${r.batch_no}（${r.beer_style}）`} onClose={onClose} width={820}>
      <div className="flex mb" style={{flexWrap:'wrap'}}>
        <span className={`badge ${r.level === '全面召回' ? 'red' : 'yellow'}`}>{r.level}</span>
        <span className={`badge ${r.status === 'active' ? 'yellow' : 'green'}`}>{r.status === 'active' ? '进行中' : '已完成'}</span>
        <span>原因：{r.reason}</span>
      </div>
      <div className="flex mb" style={{flexWrap:'wrap'}}>
        {Object.entries(RECALL_STATES).map(([k, v]) => (
          <span key={k} className="badge blue">{v}：{sc[k] || 0}</span>
        ))}
      </div>
      <ErrorBox error={error} />
      <table className="tbl">
        <thead><tr>
          <th>桶编号</th><th>召回时状态</th><th>持有方</th><th>处理状态</th><th>备注</th>{canOperate && <th>操作</th>}
        </tr></thead>
        <tbody>
          {r.items.map(it => (
            <tr key={it.id}>
              <td>{it.keg_no}</td>
              <td><span className="badge purple">{RECALL_STATES[it.state] || it.state}</span></td>
              <td>{it.owner_name || '-'}</td>
              <td><span className={`badge ${{pending:'yellow',returned:'green',destroyed:'gray',exempted:'blue'}[it.status]}`}>
                {{pending:'待处理',returned:'已退回',destroyed:'已销毁',exempted:'已豁免'}[it.status]}</span></td>
              <td className="muted">{it.note}</td>
              {canOperate && (
                <td className="flex">
                  {it.status === 'pending' && <>
                    <button className="btn small primary" onClick={() => update(it.id, 'returned')}>退回</button>
                    <button className="btn small" onClick={() => update(it.id, 'destroyed')}>销毁</button>
                    <button className="btn small" onClick={() => update(it.id, 'exempted')}>豁免</button>
                  </>}
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </Modal>
  )
}
