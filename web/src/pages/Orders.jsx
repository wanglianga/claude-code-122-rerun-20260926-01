import React, { useEffect, useState } from 'react'
import { api, fmtTime } from '../api'
import { Modal, ErrorBox, OkBox, KegStatusBadge } from '../components/ui'

const ORDER_STATUS = { pending: '待发货', delivering: '配送中', delivered: '已送达', cancelled: '已取消' }
const ORDER_COLOR = { pending: 'yellow', delivering: 'purple', delivered: 'green', cancelled: 'gray' }

export default function Orders({ user }) {
  const [list, setList] = useState([])
  const [error, setError] = useState('')
  const [ok, setOk] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [detail, setDetail] = useState(null)
  const [deliverFor, setDeliverFor] = useState(null)
  const canCreate = ['warehouse', 'distributor', 'admin'].includes(user.role)

  const load = () => api('/orders').then(setList).catch(e => setError(e.message))
  useEffect(() => { load() }, [])

  const openDetail = async id => {
    try { setDetail(await api('/orders/' + id)) } catch (e) { setError(e.message) }
  }
  const act = async (id, action, body) => {
    try {
      await api(`/orders/${id}/${action}`, { method: 'POST', body })
      setOk('操作成功'); load()
      if (detail?.id === id) openDetail(id)
    } catch (e) { setError(e.message) }
  }

  return (
    <div>
      <div className="flex-between mb">
        <h2>配送订单</h2>
        {canCreate && <button className="btn primary" onClick={() => setShowCreate(true)}>新建配送单</button>}
      </div>
      <ErrorBox error={error} onClose={() => setError('')} />
      <OkBox msg={ok} />
      <div className="card">
        <table className="tbl">
          <thead><tr>
            <th>单号</th><th>发货方</th><th>收货方</th><th>司机</th><th>桶数</th>
            <th>节日铺货</th><th>状态</th><th>收货人</th><th>创建时间</th><th>操作</th>
          </tr></thead>
          <tbody>
            {list.map(o => (
              <tr key={o.id}>
                <td><a onClick={() => openDetail(o.id)} style={{cursor:'pointer'}}>{o.order_no}</a></td>
                <td>{o.from_type === 'brewery' ? '酒厂仓库' : '经销商'}</td>
                <td>{o.to_name}</td>
                <td>{o.driver_name || '-'}</td>
                <td>{o.keg_count}</td>
                <td>{o.is_holiday_temp ? <span className="badge purple">节日临时</span> : '-'}</td>
                <td><span className={`badge ${ORDER_COLOR[o.status]}`}>{ORDER_STATUS[o.status]}</span></td>
                <td>{o.receiver_name || '-'}</td>
                <td>{fmtTime(o.created_at)}</td>
                <td className="flex">
                  <button className="btn small" onClick={() => openDetail(o.id)}>详情</button>
                  {['driver','warehouse','admin'].includes(user.role) && o.status === 'pending' &&
                    <button className="btn small primary" onClick={() => act(o.id, 'dispatch')}>发货</button>}
                  {['driver','warehouse','admin'].includes(user.role) && ['pending','delivering'].includes(o.status) &&
                    <button className="btn small primary" onClick={() => setDeliverFor(o)}>确认送达</button>}
                  {canCreate && ['pending','delivering'].includes(o.status) &&
                    <button className="btn small danger" onClick={() => { if (confirm('确认取消订单？酒桶将退回发货方库存')) act(o.id, 'cancel') }}>取消</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {list.length === 0 && <p className="muted mt">暂无订单</p>}
      </div>

      {showCreate && <CreateOrderModal user={user} onClose={() => setShowCreate(false)}
        onDone={() => { setShowCreate(false); setOk('配送单已创建'); load() }} />}
      {detail && <OrderDetailModal order={detail} onClose={() => setDetail(null)} />}
      {deliverFor && <DeliverModal order={deliverFor} onClose={() => setDeliverFor(null)}
        onDone={() => { setDeliverFor(null); setOk('已确认送达，押金已挂账'); load() }} />}
    </div>
  )
}

function CreateOrderModal({ user, onClose, onDone }) {
  const [bars, setBars] = useState([])
  const [dists, setDists] = useState([])
  const [drivers, setDrivers] = useState([])
  const [kegs, setKegs] = useState([])
  const [toType, setToType] = useState('bar')
  const [toBarId, setToBarId] = useState('')
  const [toDistId, setToDistId] = useState('')
  const [driverId, setDriverId] = useState('')
  const [selected, setSelected] = useState([])
  const [holiday, setHoliday] = useState(false)
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const isDist = user.role === 'distributor'

  useEffect(() => {
    api('/bars').then(setBars).catch(() => {})
    api('/distributors').then(setDists).catch(() => {})
    api('/drivers').then(setDrivers).catch(() => setDrivers([]))
    // 可发库存：酒厂=仓库 filled；经销商=自己的 at_distributor
    const qs = isDist ? '?status=at_distributor' : '?status=filled'
    api('/kegs' + qs).then(ks => {
      if (isDist && user.org_id) ks = ks.filter(k => k.location_type === 'distributor' && k.location_id === user.org_id)
      setKegs(ks)
    }).catch(() => {})
  }, [])

  const toggle = id => setSelected(s => s.includes(id) ? s.filter(x => x !== id) : [...s, id])
  const submit = async () => {
    if (selected.length === 0) { setError('请选择酒桶'); return }
    try {
      await api('/orders', { method: 'POST', body: {
        to_type: toType,
        to_bar_id: toType === 'bar' ? Number(toBarId) || null : null,
        to_distributor_id: toType === 'distributor' ? Number(toDistId) || null : null,
        driver_id: driverId ? Number(driverId) : null,
        keg_ids: selected, is_holiday_temp: holiday, note,
      } })
      onDone()
    } catch (e) { setError(e.message) }
  }

  return (
    <Modal title="新建配送单" onClose={onClose} width={680}>
      <ErrorBox error={error} />
      <div className="form-row">
        {!isDist && (
          <div className="form-item"><label>收货类型</label>
            <select value={toType} onChange={e => setToType(e.target.value)}>
              <option value="bar">酒吧</option><option value="distributor">经销商</option>
            </select></div>
        )}
        {toType === 'bar' && (
          <div className="form-item"><label>目标酒吧</label>
            <select value={toBarId} onChange={e => setToBarId(e.target.value)}>
              <option value="">请选择</option>
              {bars.map(b => <option key={b.id} value={b.id}>{b.name}（{b.city}）</option>)}
            </select></div>
        )}
        {toType === 'distributor' && !isDist && (
          <div className="form-item"><label>目标经销商</label>
            <select value={toDistId} onChange={e => setToDistId(e.target.value)}>
              <option value="">请选择</option>
              {dists.map(d => <option key={d.id} value={d.id}>{d.name}（{d.city}）</option>)}
            </select></div>
        )}
        <div className="form-item"><label>配送司机</label>
          <select value={driverId} onChange={e => setDriverId(e.target.value)}>
            <option value="">暂不指派</option>
            {drivers.map(d => <option key={d.id} value={d.id}>{d.name}</option>)}
          </select></div>
        <div className="form-item"><label>&nbsp;</label>
          <label style={{fontSize:13}}><input type="checkbox" checked={holiday} onChange={e => setHoliday(e.target.checked)} /> 节日临时铺货</label></div>
      </div>
      <div className="form-item"><label>备注</label><input value={note} onChange={e => setNote(e.target.value)} placeholder="如：国庆临时铺货，节后未售可退" /></div>
      <h4 className="mt mb">选择酒桶（已选 {selected.length}）</h4>
      <div style={{maxHeight:220, overflowY:'auto', border:'1px solid #e5e7eb', borderRadius:6}}>
        <table className="tbl">
          <thead><tr><th></th><th>桶编号</th><th>容量</th><th>批次</th><th>押金</th></tr></thead>
          <tbody>
            {kegs.map(k => (
              <tr key={k.id} onClick={() => toggle(k.id)} style={{cursor:'pointer'}}>
                <td><input type="checkbox" checked={selected.includes(k.id)} readOnly /></td>
                <td>{k.keg_no}</td><td>{k.capacity_liter}L</td>
                <td>{k.batch_no}（{k.beer_style}）</td><td>¥{k.deposit_amount}</td>
              </tr>
            ))}
            {kegs.length === 0 && <tr><td colSpan={5} className="muted">无可发库存酒桶</td></tr>}
          </tbody>
        </table>
      </div>
      <div className="actions"><button className="btn primary" onClick={submit}>创建配送单</button></div>
    </Modal>
  )
}

function OrderDetailModal({ order: o, onClose }) {
  return (
    <Modal title={`配送单 ${o.order_no}`} onClose={onClose} width={640}>
      <div className="flex mb" style={{flexWrap:'wrap'}}>
        <span className={`badge ${ORDER_COLOR[o.status]}`}>{ORDER_STATUS[o.status]}</span>
        {o.is_holiday_temp && <span className="badge purple">节日临时铺货</span>}
        <span>发货方：{o.from_type === 'brewery' ? '酒厂仓库' : '经销商'}</span>
        <span>收货方：{o.to_name}</span>
        <span>司机：{o.driver_name || '未指派'}</span>
      </div>
      {o.receiver_name && <p className="mb">收货人：<b>{o.receiver_name}</b>（{fmtTime(o.delivered_at)}）</p>}
      {o.note && <p className="muted mb">备注：{o.note}</p>}
      <table className="tbl">
        <thead><tr><th>桶编号</th><th>容量</th><th>批次</th><th>当前状态</th></tr></thead>
        <tbody>
          {o.items.map(it => (
            <tr key={it.keg_id}>
              <td>{it.keg_no}</td><td>{it.capacity_liter}L</td>
              <td>{it.batch_no}（{it.beer_style}）</td>
              <td><KegStatusBadge status={it.keg_status} name={it.keg_status_name} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </Modal>
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
    <Modal title={`确认送达 ${order.order_no}`} onClose={onClose}>
      <ErrorBox error={error} />
      <div className="alert info">送达后：酒桶转入收货方，押金按账期挂账。</div>
      <div className="form-item"><label>收货人（酒吧收货人/经销商签收人）</label>
        <input value={receiver} onChange={e => setReceiver(e.target.value)} placeholder="收货人姓名" /></div>
      <div className="actions"><button className="btn primary" onClick={submit}>确认送达</button></div>
    </Modal>
  )
}
