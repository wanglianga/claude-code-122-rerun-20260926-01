import React, { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'

export default function Dashboard({ user }) {
  const [d, setD] = useState(null)
  useEffect(() => { api('/dashboard').then(setD).catch(() => {}) }, [])
  if (!d) return <div className="muted">加载中…</div>
  const ks = d.keg_states || {}
  const names = d.keg_state_names || {}
  const inCirculation = (ks.delivered||0)+(ks.opened||0)+(ks.empty||0)+(ks.pickup_scheduled||0)
  return (
    <div>
      <h2 className="mb">工作台</h2>
      <div className="stat-grid">
        <div className="stat"><div className="num">{d.batch_total}</div><div className="lbl">酒液批次</div></div>
        <div className="stat"><div className="num">{inCirculation}</div><div className="lbl">酒吧在营酒桶</div></div>
        <div className="stat"><div className="num">{ks.filled||0}</div><div className="lbl">仓库可发库存</div></div>
        <div className="stat"><div className="num">{(d.pending_orders||0)+(d.delivering_orders||0)}</div><div className="lbl">进行中配送单</div></div>
        <div className="stat alert"><div className="num">{d.open_incidents}</div><div className="lbl">未结异常事件</div></div>
        <div className="stat alert"><div className="num">{d.active_recalls}</div><div className="lbl">进行中召回</div></div>
        <div className="stat"><div className="num">¥{Number(d.deposit_held||0).toLocaleString()}</div><div className="lbl">在押押金总额</div></div>
        <div className="stat alert"><div className="num">{d.overdue_deposit_tx}</div><div className="lbl">账期逾期流水</div></div>
      </div>

      {user.role === 'bar_manager' && d.my_keg_states && (
        <div className="card">
          <h3>我的酒吧概况（在押押金 ¥{Number(d.my_deposit||0).toLocaleString()}）</h3>
          <div className="flex" style={{flexWrap:'wrap'}}>
            {Object.entries(d.my_keg_states).map(([s, n]) => (
              <span key={s} className="badge blue">{names[s]||s}：{n}</span>
            ))}
            <Link to="/bar" className="btn small primary">进入我的酒吧</Link>
          </div>
        </div>
      )}

      <div className="card">
        <h3>酒桶状态分布</h3>
        <div className="flex" style={{flexWrap:'wrap'}}>
          {Object.entries(ks).map(([s, n]) => (
            <span key={s} className="badge">{names[s]||s}：{n}</span>
          ))}
        </div>
      </div>

      <div className="grid2">
        <div className="card">
          <h3>批次风险</h3>
          <p>停售批次：<b>{d.batch_suspended}</b> 个；召回批次：<b>{d.batch_recalled}</b> 个</p>
          <p className="muted mt">品控可在「批次管理」中对多家投诉批次执行停售或发起召回。</p>
        </div>
        <div className="card">
          <h3>待清洗 / 清洗队列</h3>
          <p>清洗厂待检测酒桶：<b>{d.cleaning_queue}</b> 只</p>
          <p className="muted mt">清洗合格的酒桶自动获得再灌装资格，并通知财务释放押金。</p>
        </div>
      </div>
    </div>
  )
}
