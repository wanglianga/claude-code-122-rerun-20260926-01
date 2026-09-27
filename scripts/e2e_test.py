#!/usr/bin/env python3
"""端到端业务流程验证：覆盖 配送→开桶→反馈→回收→清洗→押金→停售→召回→异常协同"""
import json, urllib.request, sys

BASE = 'http://localhost:8080/api'
tokens = {}

def call(method, path, body=None, user=None, expect_error=False):
    req = urllib.request.Request(BASE + path, method=method)
    req.add_header('Content-Type', 'application/json')
    if user:
        req.add_header('Authorization', 'Bearer ' + tokens[user])
    data = json.dumps(body).encode() if body is not None else None
    try:
        with urllib.request.urlopen(req, data) as r:
            return json.loads(r.read())
    except urllib.error.HTTPError as e:
        d = json.loads(e.read() or b'{}')
        if expect_error:
            return {'_error': d.get('error', str(e)), '_status': e.code}
        raise AssertionError(f'{method} {path} -> {e.code}: {d}')

def login(u):
    r = call('POST', '/login', {'username': u, 'password': '123456'})
    tokens[u] = r['token']
    return r['user']

passed = []
def check(name, cond, extra=''):
    assert cond, f'FAIL: {name} {extra}'
    passed.append(name)
    print(f'  ✓ {name}')

for u in ['admin','warehouse','qc','finance','cleaner','driver1','driver2','bar1','bar2','bar3','dist_hz']:
    login(u)
print('== 登录全部角色成功 ==')

# ---------- 1. 仓库：新建批次 + 灌装入库 ----------
print('== 1. 批次与酒桶 ==')
b = call('POST', '/batches', {'batch_no':'IPA-20260926','beer_style':'IPA','brew_date':'2026-09-10','fill_date':'2026-09-26','best_before':'2026-12-26','abv':6.2}, 'warehouse')
batch_id = b['id']
k1 = call('POST', '/kegs', {'keg_no':'KEG-T-001','capacity_liter':30,'batch_id':batch_id,'deposit_amount':500}, 'warehouse')['id']
k2 = call('POST', '/kegs', {'keg_no':'KEG-T-002','capacity_liter':30,'batch_id':batch_id,'deposit_amount':500}, 'warehouse')['id']
check('创建批次与两只酒桶', k1 and k2)

# ---------- 2. 配送：仓库→酒吧 ----------
print('== 2. 配送流程 ==')
o = call('POST', '/orders', {'to_type':'bar','to_bar_id':1,'driver_id':6,'keg_ids':[k1,k2],'note':'E2E测试单'}, 'warehouse')
oid = o['id']
check('创建配送单', o['order_no'].startswith('DO-'))
keg = call('GET', f'/kegs/{k1}', user='warehouse')
check('下单后桶状态=在途', keg['status'] == 'in_transit', keg['status'])
call('POST', f'/orders/{oid}/dispatch', user='driver1')
call('POST', f'/orders/{oid}/deliver', {'receiver_name':'王店长'}, 'driver1')
keg = call('GET', f'/kegs/{k1}', user='warehouse')
check('送达后桶在酒吧未开桶', keg['status'] == 'delivered' and keg['location_name'] == '麦浪精酿吧')
od = call('GET', f'/orders/{oid}', user='warehouse')
check('订单记录收货人', od['receiver_name'] == '王店长')

# 押金挂账
accts = call('GET', '/deposits/accounts', user='finance')
bar1_acct = [a for a in accts if a['owner_type']=='bar' and a['owner_id']==1][0]
txs = call('GET', f"/deposits/accounts/{bar1_acct['id']}/transactions", user='finance')
holds = [t for t in txs if t['keg_id'] in (k1,k2) and t['type']=='hold' and t['status']=='pending']
check('两只桶押金挂账(账期内)', len(holds) == 2 and all(t['due_date'] for t in holds))

# ---------- 3. 酒吧：开桶→反馈→打空→预约回收 ----------
print('== 3. 酒吧端流程 ==')
call('POST', f'/kegs/{k1}/open', user='bar1')
keg = call('GET', f'/kegs/{k1}', user='bar1')
check('开桶成功', keg['status'] == 'opened')
call('POST', '/feedbacks', {'keg_id':k1,'pour_speed':'偏慢','pressure_issue':'气压不足','taste_anomaly':'轻微','taste_note':'略氧化'}, 'bar1')
keg = call('GET', f'/kegs/{k1}', user='bar1')
check('反馈已记录到追溯', len(keg['feedbacks']) == 1)
call('POST', f'/kegs/{k1}/empty', user='bar1')
appt = call('POST', '/appointments', {'scheduled_date':'2026-09-27','note':'E2E回收'}, 'bar1')
check('创建回收预约', appt['keg_count'] >= 1, appt)
keg = call('GET', f'/kegs/{k1}', user='bar1')
check('预约后桶=已预约回收', keg['status'] == 'pickup_scheduled')

# ---------- 4. 司机：接单→扫码→完成 ----------
print('== 4. 司机回收扫码 ==')
aid = appt['id']
call('POST', f'/appointments/{aid}/claim', user='driver2')
r = call('POST', f'/appointments/{aid}/scan', {'keg_no':'KEG-T-001','appearance':'完好','residual':'无残液','cap_status':'封盖完好'}, 'driver2')
check('司机扫码成功', r['keg_id'] == k1)
keg = call('GET', f'/kegs/{k1}', user='warehouse')
check('扫码后桶=已回收待清洗', keg['status'] == 'returned')
# 漏扫检测：KEG-IPA-008 也属于该酒吧待回收，完成时应产生漏扫事件
r = call('POST', f'/appointments/{aid}/complete', user='driver2')
check('完成回收并检出漏扫', r['missed_count'] >= 1, r)
incs = call('GET', '/incidents?type=missed_scan', user='warehouse')
check('漏扫异常事件已生成', any('漏扫' in i['title'] for i in incs))

# ---------- 5. 清洗厂：检测合格→再灌装资格 ----------
print('== 5. 清洗检测 ==')
call('POST', '/cleaning', {'keg_id':k1,'result':'passed','check_note':'内窥无残留'}, 'cleaner')
keg = call('GET', f'/kegs/{k1}', user='warehouse')
check('清洗合格获得再灌装资格', keg['status'] == 'clean_passed')
call('POST', f'/kegs/{k1}/refill', {'batch_id':batch_id}, 'warehouse')
keg = call('GET', f'/kegs/{k1}', user='warehouse')
check('再灌装后回到可发库存', keg['status'] == 'filled')

# ---------- 6. 财务：按桶释放/扣减押金 ----------
print('== 6. 押金结算 ==')
call('POST', '/deposits/settle', {'keg_id':k1,'action':'release','reason':'空桶回收合格'}, 'finance')
keg = call('GET', f'/kegs/{k1}', user='finance')
types = [t['type'] for t in keg['deposit_transactions']]
check('押金释放流水生成', 'release' in types)
call('POST', '/deposits/settle', {'keg_id':k2,'action':'deduct','amount':200,'reason':'E2E扣款测试'}, 'finance')
keg = call('GET', f'/kegs/{k2}', user='finance')
types = [t['type'] for t in keg['deposit_transactions']]
check('扣款+剩余释放流水', 'deduct' in types and 'release' in types)

# ---------- 7. 品控：停售批次→多方通知 ----------
print('== 7. 批次停售通知 ==')
# 找 SOUR 批次（种子数据，bar4 有未开桶）
batches = call('GET', '/batches', user='qc')
sour = [b for b in batches if b['batch_no']=='SOUR-20260915'][0]
call('POST', f"/batches/{sour['id']}/suspend", {'reason':'E2E停售测试'}, 'qc')
# 酒吧4 应收到停售通知
login('bar4')
notifs = call('GET', '/notifications', user='bar4')
check('已收货酒吧收到停售通知', any(n['type']=='stop_sale' and 'SOUR' in n['title'] for n in notifs))
# 开桶被阻断
kegs = call('GET', '/kegs?bar_id=4&status=delivered', user='bar4')
sour_keg = [k for k in kegs if k['batch_no']=='SOUR-20260915'][0]
r = call('POST', f"/kegs/{sour_keg['id']}/open", user='bar4', expect_error=True)
check('停售批次禁止开桶', '_error' in r, r)

# ---------- 8. 召回：范围圈定 ----------
print('== 8. 批次召回 ==')
r = call('POST', '/recalls', {'batch_id':sour['id'],'reason':'E2E召回测试','level':'全面召回'}, 'qc')
recall_id = r['id']
scope = r['scope']
check('召回范围区分已售/开桶/未开桶/在库', scope.get('sold',0)>=0 and scope.get('opened',0)>=1 and scope.get('unopened',0)>=1 and scope.get('in_warehouse',0)>=1, scope)
rd = call('GET', f'/recalls/{recall_id}', user='qc')
check('召回明细生成', len(rd['items']) >= 4, len(rd['items']))
# 酒吧收到召回通知
notifs = call('GET', '/notifications', user='bar4')
check('酒吧收到召回通知', any(n['type']=='recall' for n in notifs))
# 处理一条召回明细
item = [i for i in rd['items'] if i['state']=='unopened'][0]
r = call('POST', f"/recalls/{recall_id}/items/{item['id']}", {'status':'returned','note':'E2E退回'}, 'warehouse')
check('召回明细退回处理', r['ok'])

# ---------- 9. 异常协同：多方处理同一事件 ----------
print('== 9. 异常协同 ==')
incs = call('GET', '/incidents?type=batch_complaint', user='qc')
inc = [i for i in incs if 'SOUR' in i['title']][0]
call('POST', f"/incidents/{inc['id']}/actions", {'action':'建议停售','note':'两家酒吧严重投诉'}, 'qc')
call('POST', f"/incidents/{inc['id']}/actions", {'action':'核实库存','note':'在库1桶已冻结'}, 'warehouse')
call('POST', f"/incidents/{inc['id']}/actions", {'action':'押金暂扣','note':'待批次结论'}, 'finance')
det = call('GET', f"/incidents/{inc['id']}", user='admin')
check('多方在同一事件协同', len(det['actions']) == 3 and det['status'] == 'processing')
call('POST', f"/incidents/{inc['id']}/resolve", {'resolution':'批次停售，已发起召回，押金暂缓释放'}, 'qc')
det = call('GET', f"/incidents/{inc['id']}", user='admin')
check('事件办结并记录结论', det['status'] == 'resolved' and '召回' in det['resolution'])

# ---------- 10. 经销商：库存→向酒吧铺货 ----------
print('== 10. 经销商铺货 ==')
kegs = call('GET', '/kegs?distributor_id=1&status=at_distributor', user='dist_hz')
check('经销商有库存桶', len(kegs) >= 3, len(kegs))
o = call('POST', '/orders', {'to_type':'bar','to_bar_id':1,'driver_id':6,'keg_ids':[kegs[0]['id']],'is_holiday_temp':True,'note':'国庆临时铺货'}, 'dist_hz')
check('经销商创建节日临时铺货单', o['order_no'].startswith('DO-'))

# ---------- 11. 越权与状态机防护 ----------
print('== 11. 权限与状态防护 ==')
r = call('POST', '/kegs', {'keg_no':'KEG-X','capacity_liter':30,'batch_id':batch_id}, 'bar1', expect_error=True)
check('酒吧不能灌装入库', r.get('_status') == 403)
r = call('POST', '/deposits/settle', {'keg_id':k1,'action':'release'}, 'warehouse', expect_error=True)
check('仓库不能结算押金', r.get('_status') == 403)
r = call('POST', f"/batches/{batch_id}/suspend", {'reason':'x'}, 'warehouse', expect_error=True)
check('仓库不能停售批次', r.get('_status') == 403)
r = call('POST', f'/kegs/{k2}/open', user='bar2', expect_error=True)
check('他店店长不能开桶', r.get('_status') in (403, 400), r)

print(f'\n全部通过：{len(passed)} 项验证')
