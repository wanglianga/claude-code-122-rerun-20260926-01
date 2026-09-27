import React, { useEffect, useState } from 'react'
import { api, fmtTime, fmtDate, DEPOSIT_TX_TYPES } from '../api'
import { Modal, ErrorBox, OkBox } from '../components/ui'

// 押金财务：账户 + 流水 + 按桶释放/扣减
export default function Deposits({ user }) {
  const [accounts, setAccounts] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [acctDetail, setAcctDetail] = useState(null)
  const isFinance = ['finance', 'admin'].includes(user.role)

  const load = () => api('/deposits/accounts').then(list => {
    // 酒吧店长/经销商只看自己
    if (user.role === 'bar_manager') return setAccounts(list.filter(a => a.owner_type === 'bar' && a.owner_id === user.org_id))
    if (user.role === 'distributor') return setAccounts(list.filter(a => a.owner_type === 'distributor' && a.owner_id === user.org_id))
    setAccounts(list)
  }).catch(e => setError(e.message))
  useEffect(() => { load() }, [])

  return (
    <div>
      <h2 className="mb">押金财务</h2>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />
      <div className="card">
        <h3>押金账户</h3>
        <table className="tbl">
          <thead><tr>
            <th>账户主体</th><th>类型</th><th>在押押金</th><th>账期</th><th>待结算流水</th><th>逾期</th><th>操作</th>
          </tr></thead>
          <tbody>
            {accounts.map(a => (
              <tr key={a.id}>
                <td>{a.owner_name}</td>
                <td>{a.owner_type === 'bar' ? '酒吧' : '经销商'}</td>
                <td><b>¥{Number(a.balance).toLocaleString()}</b></td>
                <td>{a.credit_term_days} 天</td>
                <td>{a.pending_count} 笔 / ¥{Number(a.pending_amount).toLocaleString()}</td>
                <td>{a.overdue_count > 0 ? <span className="badge red">{a.overdue_count} 笔逾期</span> : '-'}</td>
                <td><button className="btn small" onClick={() => setAcctDetail(a)}>流水 / 结算</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {acctDetail && <AcctModal acct={acctDetail} isFinance={isFinance}
        onClose={() => setAcctDetail(null)} onChanged={() => { load() }} />}
    </div>
  )
}

function AcctModal({ acct, isFinance, onClose, onChanged }) {
  const [txs, setTxs] = useState([])
  const [error, setError] = useState('')
  const [settleFor, setSettleFor] = useState(null)
  const load = () => api(`/deposits/accounts/${acct.id}/transactions`).then(setTxs).catch(e => setError(e.message))
  useEffect(() => { load() }, [])

  return (
    <Modal title={`${acct.owner_name} · 押金流水（在押 ¥${Number(acct.balance).toLocaleString()}）`} onClose={onClose} width={760}>
      <ErrorBox error={error} />
      <table className="tbl">
        <thead><tr>
          <th>时间</th><th>类型</th><th>酒桶</th><th>金额</th><th>状态</th><th>账期到期</th><th>事由</th><th>{isFinance ? '操作' : ''}</th>
        </tr></thead>
        <tbody>
          {txs.map(t => (
            <tr key={t.id}>
              <td>{fmtTime(t.created_at)}</td>
              <td><span className={`badge ${{hold:'yellow',release:'green',deduct:'red',adjust:'blue'}[t.type]}`}>{DEPOSIT_TX_TYPES[t.type]}</span></td>
              <td>{t.keg_no || '-'}</td>
              <td>¥{t.amount}</td>
              <td>{t.status === 'pending' ? <span className="badge yellow">待结算</span> : <span className="badge green">已结算</span>}
                {t.overdue && <span className="badge red">逾期</span>}</td>
              <td>{t.due_date ? fmtDate(t.due_date) : '-'}</td>
              <td className="muted">{t.reason}</td>
              <td>
                {isFinance && t.type === 'hold' && t.status === 'pending' && t.keg_id &&
                  <button className="btn small primary" onClick={() => setSettleFor(t)}>按桶结算</button>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {settleFor && <SettleModal tx={settleFor} onClose={() => setSettleFor(null)}
        onDone={() => { setSettleFor(null); load(); onChanged() }} />}
    </Modal>
  )
}

function SettleModal({ tx, onClose, onDone }) {
  const [action, setAction] = useState('release')
  const [amount, setAmount] = useState(tx.amount)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const submit = async () => {
    try {
      await api('/deposits/settle', { method: 'POST', body: {
        keg_id: tx.keg_id, action, amount: Number(amount), reason } })
      onDone()
    } catch (e) { setError(e.message) }
  }
  return (
    <Modal title={`按桶结算押金 · ${tx.keg_no}（挂账 ¥${tx.amount}）`} onClose={onClose}>
      <ErrorBox error={error} />
      <div className="form-item"><label>结算方式</label>
        <select value={action} onChange={e => setAction(e.target.value)}>
          <option value="release">全额释放（空桶回收合格）</option>
          <option value="deduct">扣款（丢失/变形/超期/清洗不合格等）</option>
        </select></div>
      {action === 'deduct' && (
        <div className="form-item mt"><label>扣款金额（0 ~ {tx.amount}，剩余部分自动释放）</label>
          <input type="number" value={amount} onChange={e => setAmount(e.target.value)} /></div>
      )}
      <div className="form-item mt"><label>事由</label>
        <textarea rows={2} value={reason} onChange={e => setReason(e.target.value)}
          placeholder={action === 'release' ? '如：空桶回收合格，押金释放' : '如：桶体变形扣款'} /></div>
      <div className="actions"><button className="btn primary" onClick={submit}>确认结算</button></div>
    </Modal>
  )
}
