package main

import (
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
)

// seedIfEmpty 首次启动时写入演示数据
func seedIfEmpty() {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM users`).Scan(&n); err != nil || n > 0 {
		return
	}
	log.Println("seeding demo data...")
	hash := func(pw string) string {
		b, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		return string(b)
	}
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("seed begin: %v", err)
	}
	defer tx.Rollback()

	// ---------- 用户 ----------
	type usr struct{ username, name, role string }
	users := []usr{
		{"admin", "系统管理员", "admin"},
		{"warehouse", "张满仓(酒厂仓库)", "warehouse"},
		{"qc", "陈品控(品控)", "qc"},
		{"finance", "刘财务(财务)", "finance"},
		{"cleaner", "王清洗(清洗厂)", "cleaner"},
		{"driver1", "李司机", "driver"},
		{"driver2", "赵司机", "driver"},
	}
	uid := map[string]int{}
	for _, u := range users {
		var id int
		if err := tx.QueryRow(`INSERT INTO users(username,password_hash,name,role) VALUES($1,$2,$3,$4) RETURNING id`,
			u.username, hash("123456"), u.name, u.role).Scan(&id); err != nil {
			log.Fatalf("seed user: %v", err)
		}
		uid[u.username] = id
	}

	// ---------- 经销商（多城市） ----------
	var distHZ, distSH int
	tx.QueryRow(`INSERT INTO distributors(name,city,contact,phone) VALUES('西湖酒业经销','杭州','沈经理','13800000001') RETURNING id`).Scan(&distHZ)
	tx.QueryRow(`INSERT INTO distributors(name,city,contact,phone) VALUES('浦江精酿供应链','上海','陆经理','13800000002') RETURNING id`).Scan(&distSH)
	for _, du := range []struct{ username, name string; org int }{
		{"dist_hz", "沈经销(杭州)", distHZ},
		{"dist_sh", "陆经销(上海)", distSH},
	} {
		var id int
		tx.QueryRow(`INSERT INTO users(username,password_hash,name,role,org_id) VALUES($1,$2,$3,'distributor',$4) RETURNING id`,
			du.username, hash("123456"), du.name, du.org).Scan(&id)
		uid[du.username] = id
		tx.Exec(`INSERT INTO deposit_accounts(owner_type,owner_id) VALUES('distributor',$1)`, du.org)
	}

	// ---------- 酒吧 ----------
	type barT struct {
		name, city, addr, mgr string
		dist                  *int
		term                  int
	}
	bars := []barT{
		{"麦浪精酿吧", "杭州", "西湖区文三路 100 号", "王店长", &distHZ, 30},
		{"泡沫实验室", "杭州", "滨江区江南大道 88 号", "赵店长", nil, 45},
		{"麦芽仓库", "上海", "静安区愚园路 200 号", "钱店长", &distSH, 30},
		{"酸角酒馆", "上海", "徐汇区衡山路 12 号", "孙店长", nil, 15},
	}
	barIDs := []int{}
	for i, b := range bars {
		var id int
		if err := tx.QueryRow(`INSERT INTO bars(name,city,address,manager_name,phone,distributor_id,deposit_credit_term_days)
			VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			b.name, b.city, b.addr, b.mgr, fmt.Sprintf("1390000000%d", i+1), b.dist, b.term).Scan(&id); err != nil {
			log.Fatalf("seed bar: %v", err)
		}
		barIDs = append(barIDs, id)
		tx.Exec(`INSERT INTO deposit_accounts(owner_type,owner_id) VALUES('bar',$1)`, id)
		var mid int
		tx.QueryRow(`INSERT INTO users(username,password_hash,name,role,org_id) VALUES($1,$2,$3,'bar_manager',$4) RETURNING id`,
			fmt.Sprintf("bar%d", i+1), hash("123456"), b.mgr+"("+b.name+")", id).Scan(&mid)
		uid[fmt.Sprintf("bar%d", i+1)] = mid
	}
	bar1, bar2, bar3, bar4 := barIDs[0], barIDs[1], barIDs[2], barIDs[3]

	// ---------- 批次 ----------
	type batchT struct {
		no, style, brew, fill, bb string
		abv                       float64
	}
	batches := []batchT{
		{"IPA-20260901", "IPA", "2026-08-20", "2026-09-01", "2026-11-30", 6.5},
		{"STOUT-20260905", "世涛", "2026-08-25", "2026-09-05", "2027-03-05", 8.0},
		{"WHEAT-20260910", "小麦", "2026-08-30", "2026-09-10", "2026-12-10", 5.2},
		{"SOUR-20260915", "酸啤", "2026-09-01", "2026-09-15", "2026-12-15", 4.8},
	}
	bid := map[string]int{}
	for _, b := range batches {
		var id int
		tx.QueryRow(`INSERT INTO batches(batch_no,beer_style,brew_date,fill_date,best_before,abv)
			VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, b.no, b.style, b.brew, b.fill, b.bb, b.abv).Scan(&id)
		bid[b.no] = id
	}
	ipa, stout, wheat, sour := bid["IPA-20260901"], bid["STOUT-20260905"], bid["WHEAT-20260910"], bid["SOUR-20260915"]

	// ---------- 酒桶 ----------
	type kegT struct {
		no      string
		cap     float64
		batch   int
		dep     float64
		status  string
		locType string
		locID   int
	}
	kegs := []kegT{
		{"KEG-IPA-001", 30, ipa, 500, "filled", "warehouse", 0},
		{"KEG-IPA-002", 30, ipa, 500, "filled", "warehouse", 0},
		{"KEG-IPA-003", 20, ipa, 400, "filled", "warehouse", 0},
		{"KEG-IPA-004", 20, ipa, 400, "filled", "warehouse", 0},
		{"KEG-IPA-005", 30, ipa, 500, "delivered", "bar", bar1},
		{"KEG-IPA-006", 30, ipa, 500, "delivered", "bar", bar1},
		{"KEG-IPA-007", 30, ipa, 500, "opened", "bar", bar1},
		{"KEG-IPA-008", 30, ipa, 500, "pickup_scheduled", "bar", bar1},
		{"KEG-IPA-009", 30, ipa, 500, "lost", "bar", bar2},
		{"KEG-IPA-010", 30, ipa, 500, "delivered", "bar", bar2}, // 超期未还（delivered_at 35 天前）
		{"KEG-IPA-011", 30, ipa, 500, "in_transit", "in_transit", 0}, // 国庆临时铺货待发货
		{"KEG-IPA-012", 30, ipa, 500, "in_transit", "in_transit", 0},
		{"KEG-STOUT-001", 20, stout, 600, "at_distributor", "distributor", distHZ},
		{"KEG-STOUT-002", 20, stout, 600, "at_distributor", "distributor", distHZ},
		{"KEG-STOUT-003", 20, stout, 600, "at_distributor", "distributor", distHZ},
		{"KEG-STOUT-004", 20, stout, 600, "delivered", "bar", bar3},
		{"KEG-STOUT-005", 20, stout, 600, "opened", "bar", bar3},
		{"KEG-STOUT-006", 20, stout, 600, "clean_failed", "cleaning_plant", 0},
		{"KEG-WHEAT-001", 30, wheat, 450, "in_transit", "in_transit", 0},
		{"KEG-WHEAT-002", 30, wheat, 450, "in_transit", "in_transit", 0},
		{"KEG-WHEAT-003", 30, wheat, 450, "empty", "bar", bar2},
		{"KEG-WHEAT-004", 30, wheat, 450, "returned", "cleaning_plant", 0},
		{"KEG-WHEAT-005", 30, wheat, 450, "clean_passed", "warehouse", 0},
		{"KEG-SOUR-001", 20, sour, 500, "opened", "bar", bar1},
		{"KEG-SOUR-002", 20, sour, 500, "opened", "bar", bar3},
		{"KEG-SOUR-003", 20, sour, 500, "delivered", "bar", bar4},
		{"KEG-SOUR-004", 20, sour, 500, "filled", "warehouse", 0},
	}
	kid := map[string]int{}
	for _, k := range kegs {
		var id int
		var loc interface{}
		if k.locID == 0 {
			loc = nil
		} else {
			loc = k.locID
		}
		if err := tx.QueryRow(`INSERT INTO kegs(keg_no,capacity_liter,batch_id,deposit_amount,status,location_type,location_id)
			VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			k.no, k.cap, k.batch, k.dep, k.status, k.locType, loc).Scan(&id); err != nil {
			log.Fatalf("seed keg: %v", err)
		}
		kid[k.no] = id
	}
	// 时间修正：开桶/打空/送达时间
	tx.Exec(`UPDATE kegs SET delivered_at=now()-interval '10 days' WHERE keg_no IN ('KEG-IPA-005','KEG-IPA-006','KEG-IPA-007','KEG-IPA-008')`)
	tx.Exec(`UPDATE kegs SET opened_at=now()-interval '6 days' WHERE keg_no='KEG-IPA-007'`)
	tx.Exec(`UPDATE kegs SET opened_at=now()-interval '9 days', emptied_at=now()-interval '2 days' WHERE keg_no='KEG-IPA-008'`)
	tx.Exec(`UPDATE kegs SET delivered_at=now()-interval '35 days' WHERE keg_no='KEG-IPA-010'`)
	tx.Exec(`UPDATE kegs SET delivered_at=now()-interval '5 days' WHERE keg_no IN ('KEG-STOUT-004','KEG-STOUT-005')`)
	tx.Exec(`UPDATE kegs SET opened_at=now()-interval '3 days' WHERE keg_no='KEG-STOUT-005'`)
	tx.Exec(`UPDATE kegs SET delivered_at=now()-interval '12 days', opened_at=now()-interval '8 days', emptied_at=now()-interval '4 days' WHERE keg_no='KEG-WHEAT-003'`)
	tx.Exec(`UPDATE kegs SET delivered_at=now()-interval '8 days', opened_at=now()-interval '5 days' WHERE keg_no IN ('KEG-SOUR-001','KEG-SOUR-002')`)
	tx.Exec(`UPDATE kegs SET delivered_at=now()-interval '2 days' WHERE keg_no='KEG-SOUR-003'`)

	// ---------- 配送订单 ----------
	var order1, order2, order3, order4 int
	tx.QueryRow(`INSERT INTO delivery_orders(order_no,from_type,to_type,to_bar_id,driver_id,status,receiver_name,created_by,created_at,delivered_at)
		VALUES('DO-20260916-0001','brewery','bar',$1,$2,'delivered','王店长',$3,now()-interval '10 days',now()-interval '10 days') RETURNING id`,
		bar1, uid["driver1"], uid["warehouse"]).Scan(&order1)
	for _, kn := range []string{"KEG-IPA-005", "KEG-IPA-006", "KEG-IPA-007", "KEG-IPA-008"} {
		tx.Exec(`INSERT INTO delivery_order_items(order_id,keg_id) VALUES($1,$2)`, order1, kid[kn])
		tx.Exec(`UPDATE kegs SET current_order_id=$1 WHERE id=$2`, order1, kid[kn])
	}
	tx.QueryRow(`INSERT INTO delivery_orders(order_no,from_type,from_distributor_id,to_type,to_bar_id,driver_id,status,receiver_name,created_by,created_at,delivered_at)
		VALUES('DO-20260921-0002','distributor',$1,'bar',$2,$3,'delivered','钱店长',$4,now()-interval '5 days',now()-interval '5 days') RETURNING id`,
		distSH, bar3, uid["driver2"], uid["dist_sh"]).Scan(&order2)
	for _, kn := range []string{"KEG-STOUT-004", "KEG-STOUT-005"} {
		tx.Exec(`INSERT INTO delivery_order_items(order_id,keg_id) VALUES($1,$2)`, order2, kid[kn])
	}
	tx.QueryRow(`INSERT INTO delivery_orders(order_no,from_type,to_type,to_bar_id,driver_id,status,receiver_name,created_by,created_at,delivered_at)
		VALUES('DO-20260822-0003','brewery','bar',$1,$2,'delivered','赵店长',$3,now()-interval '35 days',now()-interval '35 days') RETURNING id`,
		bar2, uid["driver1"], uid["warehouse"]).Scan(&order3)
	tx.Exec(`INSERT INTO delivery_order_items(order_id,keg_id) VALUES($1,$2)`, order3, kid["KEG-IPA-010"])
	tx.QueryRow(`INSERT INTO delivery_orders(order_no,from_type,to_type,to_bar_id,driver_id,status,is_holiday_temp,note,created_by)
		VALUES('DO-20260925-0004','brewery','bar',$1,$2,'pending',TRUE,'国庆黄金周临时铺货，10月7日后未售出可退',$3) RETURNING id`,
		bar4, uid["driver1"], uid["warehouse"]).Scan(&order4)
	for _, kn := range []string{"KEG-IPA-011", "KEG-IPA-012"} {
		tx.Exec(`INSERT INTO delivery_order_items(order_id,keg_id) VALUES($1,$2)`, order4, kid[kn])
		tx.Exec(`UPDATE kegs SET current_order_id=$1 WHERE id=$2`, order4, kid[kn])
	}
	// 在途订单（小麦→泡沫实验室）
	var order5 int
	tx.QueryRow(`INSERT INTO delivery_orders(order_no,from_type,to_type,to_bar_id,driver_id,status,created_by)
		VALUES('DO-20260926-0005','brewery','bar',$1,$2,'delivering',$3) RETURNING id`,
		bar2, uid["driver1"], uid["warehouse"]).Scan(&order5)
	for _, kn := range []string{"KEG-WHEAT-001", "KEG-WHEAT-002"} {
		tx.Exec(`INSERT INTO delivery_order_items(order_id,keg_id) VALUES($1,$2)`, order5, kid[kn])
		tx.Exec(`UPDATE kegs SET current_order_id=$1 WHERE id=$2`, order5, kid[kn])
	}

	// ---------- 开桶反馈 ----------
	type fbT struct {
		keg, bar, batch                                     int
		pour, pressure, taste, tasteNote, compNote          string
		complaint                                           bool
	}
	fbs := []fbT{
		{kid["KEG-IPA-007"], bar1, ipa, "正常", "正常", "无", "", "", false},
		{kid["KEG-SOUR-001"], bar1, sour, "偏慢", "气压不足", "明显", "有明显氧化味，像纸板味", "客人喝了半杯要求退单，已免单 2 杯", true},
		{kid["KEG-SOUR-002"], bar3, sour, "不出酒", "气压不足", "严重", "酸败味刺鼻，完全无法售卖", "多位客人投诉，要求退桶赔偿", true},
		{kid["KEG-STOUT-005"], bar3, stout, "正常", "正常", "无", "", "", false},
	}
	for _, f := range fbs {
		tx.Exec(`INSERT INTO feedbacks(keg_id,bar_id,batch_id,pour_speed,pressure_issue,taste_anomaly,taste_note,complaint,complaint_note,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			f.keg, f.bar, f.batch, f.pour, f.pressure, f.taste, f.tasteNote, f.complaint, f.compNote, uid["bar1"])
	}

	// ---------- 回收预约 ----------
	var appt1 int
	tx.QueryRow(`INSERT INTO pickup_appointments(appt_no,bar_id,scheduled_date,note,created_by)
		VALUES('PK-20260926-0001',$1,CURRENT_DATE+1,'打空 IPA 桶一只，请安排回收',$2) RETURNING id`,
		bar1, uid["bar1"]).Scan(&appt1)

	// ---------- 押金流水 ----------
	acctOf := func(ownerType string, ownerID int) int {
		var id int
		tx.QueryRow(`SELECT id FROM deposit_accounts WHERE owner_type=$1 AND owner_id=$2`, ownerType, ownerID).Scan(&id)
		return id
	}
	hold := func(ownerType string, ownerID, kegID int, amount float64, daysAgo int, termDays int) {
		acct := acctOf(ownerType, ownerID)
		tx.Exec(`INSERT INTO deposit_transactions(account_id,keg_id,type,amount,reason,ref_type,status,due_date,created_by,created_at)
			VALUES($1,$2,'hold',$3,'配送收货押金挂账','order','pending',(CURRENT_DATE + ($4::int - $5::int) * INTERVAL '1 day')::date,$6,now() - $5::int * INTERVAL '1 day')`,
			acct, kegID, amount, termDays, daysAgo, uid["finance"])
		tx.Exec(`UPDATE deposit_accounts SET balance=balance+$1 WHERE id=$2`, amount, acct)
	}
	// 酒吧押金挂账
	for _, kn := range []string{"KEG-IPA-005", "KEG-IPA-006", "KEG-IPA-007", "KEG-IPA-008"} {
		hold("bar", bar1, kid[kn], 500, 10, 30)
	}
	hold("bar", bar1, kid["KEG-SOUR-001"], 500, 8, 30)
	hold("bar", bar2, kid["KEG-IPA-010"], 500, 35, 45) // 未逾期（45 天账期）
	hold("bar", bar2, kid["KEG-WHEAT-003"], 450, 12, 45)
	hold("bar", bar3, kid["KEG-STOUT-004"], 600, 5, 30)
	hold("bar", bar3, kid["KEG-STOUT-005"], 600, 5, 30)
	hold("bar", bar3, kid["KEG-SOUR-002"], 500, 8, 30)
	hold("bar", bar4, kid["KEG-SOUR-003"], 500, 2, 15)
	// 经销商押金挂账
	for _, kn := range []string{"KEG-STOUT-001", "KEG-STOUT-002", "KEG-STOUT-003"} {
		hold("distributor", distHZ, kid[kn], 600, 15, 30)
	}
	// 已完成循环示例：WHEAT-005 清洗合格，押金已释放（settled）
	{
		acct := acctOf("bar", bar2)
		tx.Exec(`INSERT INTO deposit_transactions(account_id,keg_id,type,amount,reason,ref_type,status,due_date,created_by,created_at)
			VALUES($1,$2,'hold',450,'配送收货押金挂账','order','settled',CURRENT_DATE-10,$3,now()-interval '40 days')`,
			acct, kid["KEG-WHEAT-005"], uid["finance"])
		tx.Exec(`INSERT INTO deposit_transactions(account_id,keg_id,type,amount,reason,ref_type,status,created_by,created_at)
			VALUES($1,$2,'release',450,'空桶回收合格，押金释放','manual','settled',$3,now()-interval '5 days')`,
			acct, kid["KEG-WHEAT-005"], uid["finance"])
	}

	// ---------- 清洗记录 ----------
	tx.Exec(`INSERT INTO cleaning_records(keg_id,operator_id,result,check_note,created_at)
		VALUES($1,$2,'passed','内窥无残留，残碱合格，无异味',now()-interval '5 days')`, kid["KEG-WHEAT-005"], uid["cleaner"])
	tx.Exec(`INSERT INTO cleaning_records(keg_id,operator_id,result,check_note,created_at)
		VALUES($1,$2,'failed','内壁有明显划痕且残碱超标，建议报废',now()-interval '1 days')`, kid["KEG-STOUT-006"], uid["cleaner"])

	// ---------- 酒桶事件（追溯时间线示例） ----------
	ev := func(kegID int, event, detail string, actorID int) {
		tx.Exec(`INSERT INTO keg_events(keg_id,event,detail,actor_id) VALUES($1,$2,$3,$4)`, kegID, event, detail, actorID)
	}
	for _, kn := range []string{"KEG-IPA-005", "KEG-IPA-006", "KEG-IPA-007", "KEG-IPA-008"} {
		ev(kid[kn], "灌装入库", "批次 IPA-20260901", uid["warehouse"])
		ev(kid[kn], "出库配送", "订单 DO-20260916-0001", uid["warehouse"])
		ev(kid[kn], "送达签收", "收货人：王店长", uid["driver1"])
	}
	ev(kid["KEG-IPA-007"], "开桶", "", uid["bar1"])
	ev(kid["KEG-IPA-008"], "开桶", "", uid["bar1"])
	ev(kid["KEG-IPA-008"], "打空", "", uid["bar1"])
	ev(kid["KEG-IPA-009"], "登记丢失", "酒吧盘点发现少桶", uid["bar2"])
	ev(kid["KEG-STOUT-006"], "回收扫码", "外观:轻微磕碰 残液:少量 封盖:封盖完好", uid["driver2"])
	ev(kid["KEG-STOUT-006"], "清洗检测", "清洗不合格。内壁有明显划痕且残碱超标", uid["cleaner"])
	ev(kid["KEG-WHEAT-005"], "清洗检测", "清洗合格。内窥无残留", uid["cleaner"])

	// ---------- 异常事件 ----------
	var inc1, inc2 int
	tx.QueryRow(`INSERT INTO incidents(incident_no,type,keg_id,bar_id,title,description,status)
		VALUES('INC-20260920-0001','lost',$1,$2,'酒桶 KEG-IPA-009 丢失','酒吧盘点发现少桶，请财务评估押金扣款，仓库核减库存。','processing') RETURNING id`,
		kid["KEG-IPA-009"], bar2).Scan(&inc1)
	tx.Exec(`INSERT INTO incident_actions(incident_id,user_id,role,action,note) VALUES($1,$2,'warehouse','核实库存','仓库已核对出入库记录，确认该桶在酒吧环节丢失')`, inc1, uid["warehouse"])
	tx.Exec(`INSERT INTO incident_actions(incident_id,user_id,role,action,note) VALUES($1,$2,'bar_manager','情况说明','店内监控排查中，初步判断为外摆区遗失')`, inc1, uid["bar2"])
	tx.QueryRow(`INSERT INTO incidents(incident_no,type,keg_id,batch_id,title,description,status)
		VALUES('INC-20260925-0002','cleaning_failed',$1,$2,'酒桶 KEG-STOUT-006 清洗检测不合格','清洗厂检测不合格：内壁有明显划痕且残碱超标，建议报废。请评估重新清洗或报废，并确认是否影响再灌装资格。','open') RETURNING id`,
		kid["KEG-STOUT-006"], stout).Scan(&inc2)

	if err := tx.Commit(); err != nil {
		log.Fatalf("seed commit: %v", err)
	}

	// 批次投诉自动检测（酸啤两家酒吧严重反馈 → 自动生成事件）
	checkBatchComplaints(sour)
	checkLateReturns() // KEG-IPA-010 超期 35 天 → 但账期 45 天未逾期；延迟归还按 30 天规则仍会触发
	log.Println("seed done")
}
