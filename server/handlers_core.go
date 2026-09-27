package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ---------- 基础数据 ----------

func listBars(c *gin.Context) {
	rows, err := db.Query(`SELECT b.id,b.name,b.city,b.address,b.manager_name,b.phone,
		b.distributor_id, coalesce(d.name,''), b.deposit_credit_term_days
		FROM bars b LEFT JOIN distributors d ON d.id=b.distributor_id ORDER BY b.id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, term int
		var name, city, addr, mgr, phone, dname string
		var did *int
		rows.Scan(&id, &name, &city, &addr, &mgr, &phone, &did, &dname, &term)
		list = append(list, gin.H{"id": id, "name": name, "city": city, "address": addr,
			"manager_name": mgr, "phone": phone, "distributor_id": did, "distributor_name": dname,
			"deposit_credit_term_days": term})
	}
	c.JSON(http.StatusOK, list)
}

func createBar(c *gin.Context) {
	var req struct {
		Name         string `json:"name"`
		City         string `json:"city"`
		Address      string `json:"address"`
		ManagerName  string `json:"manager_name"`
		Phone        string `json:"phone"`
		DistributorID *int  `json:"distributor_id"`
		CreditTerm   int    `json:"deposit_credit_term_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.City == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称和城市必填"})
		return
	}
	if req.CreditTerm <= 0 {
		req.CreditTerm = 30
	}
	var id int
	err := db.QueryRow(`INSERT INTO bars(name,city,address,manager_name,phone,distributor_id,deposit_credit_term_days)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		req.Name, req.City, req.Address, req.ManagerName, req.Phone, req.DistributorID, req.CreditTerm).Scan(&id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ensureDepositAccount(nil, "bar", id)
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func listDistributors(c *gin.Context) {
	rows, err := db.Query(`SELECT id,name,city,contact,phone FROM distributors ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id int
		var name, city, contact, phone string
		rows.Scan(&id, &name, &city, &contact, &phone)
		// 经销商库存桶数
		var stock int
		db.QueryRow(`SELECT count(*) FROM kegs WHERE location_type='distributor' AND location_id=$1 AND status='at_distributor'`, id).Scan(&stock)
		list = append(list, gin.H{"id": id, "name": name, "city": city, "contact": contact, "phone": phone, "stock_kegs": stock})
	}
	c.JSON(http.StatusOK, list)
}

func createDistributor(c *gin.Context) {
	var req struct {
		Name, City, Contact, Phone string
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.City == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称和城市必填"})
		return
	}
	var id int
	err := db.QueryRow(`INSERT INTO distributors(name,city,contact,phone) VALUES($1,$2,$3,$4) RETURNING id`,
		req.Name, req.City, req.Contact, req.Phone).Scan(&id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ensureDepositAccount(nil, "distributor", id)
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func listUsers(c *gin.Context) {
	rows, err := db.Query(`SELECT id,username,name,role,org_id,phone FROM users ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id int
		var un, name, r, phone string
		var org *int
		rows.Scan(&id, &un, &name, &r, &org, &phone)
		list = append(list, gin.H{"id": id, "username": un, "name": name, "role": r, "org_id": org, "phone": phone})
	}
	c.JSON(http.StatusOK, list)
}

// listDrivers 司机列表（配送单指派用，仓库/经销商可用）
func listDrivers(c *gin.Context) {
	rows, err := db.Query(`SELECT id,name,phone FROM users WHERE role='driver' ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id int
		var name, phone string
		rows.Scan(&id, &name, &phone)
		list = append(list, gin.H{"id": id, "name": name, "phone": phone})
	}
	c.JSON(http.StatusOK, list)
}

// ---------- 批次 ----------

func listBatches(c *gin.Context) {
	rows, err := db.Query(`SELECT b.id,b.batch_no,b.beer_style,b.brew_date,b.fill_date,b.best_before,b.abv,b.status,b.stop_reason,
		(SELECT count(*) FROM kegs k WHERE k.batch_id=b.id) AS keg_count,
		(SELECT count(*) FROM feedbacks f WHERE f.batch_id=b.id AND (f.taste_anomaly IN ('明显','严重') OR f.complaint)) AS bad_fb
		FROM batches b ORDER BY b.id DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, kegCount, badFb int
		var no, style, status, reason string
		var brew, fill, bb string
		var abv float64
		rows.Scan(&id, &no, &style, &brew, &fill, &bb, &abv, &status, &reason, &kegCount, &badFb)
		list = append(list, gin.H{"id": id, "batch_no": no, "beer_style": style,
			"brew_date": brew[:10], "fill_date": fill[:10], "best_before": bb[:10], "abv": abv,
			"status": status, "stop_reason": reason, "keg_count": kegCount, "bad_feedback_count": badFb})
	}
	c.JSON(http.StatusOK, list)
}

func createBatch(c *gin.Context) {
	var req struct {
		BatchNo    string  `json:"batch_no"`
		BeerStyle  string  `json:"beer_style"`
		BrewDate   string  `json:"brew_date"`
		FillDate   string  `json:"fill_date"`
		BestBefore string  `json:"best_before"`
		Abv        float64 `json:"abv"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BatchNo == "" || req.BeerStyle == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "批次号和酒型必填"})
		return
	}
	var id int
	err := db.QueryRow(`INSERT INTO batches(batch_no,beer_style,brew_date,fill_date,best_before,abv)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		req.BatchNo, req.BeerStyle, req.BrewDate, req.FillDate, req.BestBefore, req.Abv).Scan(&id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "创建失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func batchDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var no, style, status, reason string
	var brew, fill, bb string
	var abv float64
	var stopAt *string
	err := db.QueryRow(`SELECT batch_no,beer_style,brew_date,fill_date,best_before,abv,status,stop_reason,stop_sale_at
		FROM batches WHERE id=$1`, id).Scan(&no, &style, &brew, &fill, &bb, &abv, &status, &reason, &stopAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "批次不存在"})
		return
	}
	// 桶状态分布
	stateRows, _ := db.Query(`SELECT status, count(*) FROM kegs WHERE batch_id=$1 GROUP BY status`, id)
	states := map[string]int{}
	if stateRows != nil {
		defer stateRows.Close()
		for stateRows.Next() {
			var s string
			var n int
			stateRows.Scan(&s, &n)
			states[s] = n
		}
	}
	// 反馈汇总
	fbRows, _ := db.Query(`SELECT f.id, k.keg_no, b.name, f.pour_speed, f.pressure_issue, f.taste_anomaly,
		f.taste_note, f.complaint, f.complaint_note, f.created_at
		FROM feedbacks f JOIN kegs k ON k.id=f.keg_id JOIN bars b ON b.id=f.bar_id
		WHERE f.batch_id=$1 ORDER BY f.id DESC`, id)
	fbs := []gin.H{}
	if fbRows != nil {
		defer fbRows.Close()
		for fbRows.Next() {
			var fid int
			var kegNo, barName, ps, pi, ta, tn, cn, ts string
			var comp bool
			fbRows.Scan(&fid, &kegNo, &barName, &ps, &pi, &ta, &tn, &comp, &cn, &ts)
			fbs = append(fbs, gin.H{"id": fid, "keg_no": kegNo, "bar_name": barName, "pour_speed": ps,
				"pressure_issue": pi, "taste_anomaly": ta, "taste_note": tn, "complaint": comp,
				"complaint_note": cn, "created_at": ts})
		}
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "batch_no": no, "beer_style": style,
		"brew_date": brew[:10], "fill_date": fill[:10], "best_before": bb[:10], "abv": abv,
		"status": status, "stop_reason": reason, "stop_sale_at": stopAt,
		"keg_states": states, "feedbacks": fbs})
}

// suspendBatch 停售批次：通知已收货酒吧（未开桶）、待配送订单、经销商库存
func suspendBatch(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Reason string `json:"reason"`
	}
	c.ShouldBindJSON(&req)
	if req.Reason == "" {
		req.Reason = "品控评估暂停销售"
	}
	res, err := db.Exec(`UPDATE batches SET status='suspended', stop_reason=$1, stop_sale_at=now()
		WHERE id=$2 AND status='normal'`, req.Reason, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "批次不存在或已停售/召回"})
		return
	}
	var batchNo string
	db.QueryRow(`SELECT batch_no FROM batches WHERE id=$1`, id).Scan(&batchNo)

	// 通知持有未开桶的酒吧
	barRows, _ := db.Query(`SELECT DISTINCT location_id FROM kegs
		WHERE batch_id=$1 AND location_type='bar' AND status IN ('delivered','opened','empty')`, id)
	if barRows != nil {
		for barRows.Next() {
			var barID int
			barRows.Scan(&barID)
			bid := barID
			notify(nil, "", &bid, "stop_sale",
				fmt.Sprintf("批次 %s 已停售，请勿再开桶售卖", batchNo),
				"停售原因："+req.Reason+"。未开桶酒桶请勿开桶，等待召回或进一步通知。", "batch", &id)
		}
		barRows.Close()
	}
	// 通知有待配送订单的司机与仓库
	orderRows, _ := db.Query(`SELECT DISTINCT o.id, o.order_no, o.driver_id FROM delivery_orders o
		JOIN delivery_order_items i ON i.order_id=o.id
		JOIN kegs k ON k.id=i.keg_id
		WHERE k.batch_id=$1 AND o.status IN ('pending','delivering')`, id)
	if orderRows != nil {
		for orderRows.Next() {
			var oid, driverID int
			var ono string
			var drv *int
			orderRows.Scan(&oid, &ono, &drv)
			driverID = 0
			if drv != nil {
				driverID = *drv
			}
			notifyRole("warehouse", "stop_sale", fmt.Sprintf("订单 %s 含停售批次 %s，请暂停发货", ono, batchNo),
				"停售原因："+req.Reason, "order", &oid)
			if driverID != 0 {
				d := driverID
				notify(&d, "", nil, "stop_sale", fmt.Sprintf("订单 %s 含停售批次 %s，请暂停配送", ono, batchNo),
					"停售原因："+req.Reason, "order", &oid)
			}
		}
		orderRows.Close()
	}
	// 通知持有库存的经销商
	distRows, _ := db.Query(`SELECT DISTINCT location_id FROM kegs
		WHERE batch_id=$1 AND location_type='distributor' AND status='at_distributor'`, id)
	if distRows != nil {
		for distRows.Next() {
			var did int
			distRows.Scan(&did)
			var uid *int
			db.QueryRow(`SELECT id FROM users WHERE role='distributor' AND org_id=$1 LIMIT 1`, did).Scan(&uid)
			if uid != nil {
				notify(uid, "", nil, "stop_sale", fmt.Sprintf("批次 %s 已停售，请冻结库存", batchNo),
					"停售原因："+req.Reason+"。请勿继续向酒吧铺货。", "batch", &id)
			}
		}
		distRows.Close()
	}
	notifyRole("finance", "stop_sale", fmt.Sprintf("批次 %s 已停售", batchNo), "停售原因："+req.Reason, "batch", &id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func resumeBatch(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	res, err := db.Exec(`UPDATE batches SET status='normal', stop_reason='', stop_sale_at=NULL
		WHERE id=$1 AND status='suspended'`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "批次不在停售状态"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 酒桶 ----------

var kegStatusNames = map[string]string{
	"filled": "在库已灌装", "in_transit": "配送在途", "at_distributor": "经销商库存",
	"delivered": "酒吧未开桶", "opened": "已开桶", "empty": "已打空待回收",
	"pickup_scheduled": "已预约回收", "returned": "已回收待清洗",
	"clean_passed": "清洗合格", "clean_failed": "清洗不合格", "lost": "丢失", "scrapped": "报废",
}

func listKegs(c *gin.Context) {
	q := `SELECT k.id,k.keg_no,k.capacity_liter,k.batch_id,coalesce(b.batch_no,''),coalesce(b.beer_style,''),
		k.deposit_amount,k.status,k.location_type,coalesce(k.location_id,0),
		coalesce(k.opened_at::text,''),coalesce(k.delivered_at::text,'')
		FROM kegs k LEFT JOIN batches b ON b.id=k.batch_id WHERE 1=1`
	args := []interface{}{}
	n := 0
	add := func(cond string, v interface{}) {
		n++
		q += fmt.Sprintf(" AND %s$%d", cond, n)
		args = append(args, v)
	}
	if s := c.Query("status"); s != "" {
		add("k.status=", s)
	}
	if s := c.Query("batch_id"); s != "" {
		add("k.batch_id=", s)
	}
	if s := c.Query("bar_id"); s != "" {
		q += fmt.Sprintf(" AND k.location_type='bar' AND k.location_id=$%d", n+1)
		n++
		args = append(args, s)
	}
	if s := c.Query("distributor_id"); s != "" {
		q += fmt.Sprintf(" AND k.location_type='distributor' AND k.location_id=$%d", n+1)
		n++
		args = append(args, s)
	}
	if s := c.Query("q"); s != "" {
		add("k.keg_no ILIKE ", "%"+s+"%")
	}
	q += " ORDER BY k.id DESC LIMIT 500"
	rows, err := db.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, locID int
		var batchID *int
		var kegNo, batchNo, style, status, locType, openedAt, deliveredAt string
		var cap float64
		var dep float64
		rows.Scan(&id, &kegNo, &cap, &batchID, &batchNo, &style, &dep, &status, &locType, &locID, &openedAt, &deliveredAt)
		locName := locationName(locType, locID)
		list = append(list, gin.H{"id": id, "keg_no": kegNo, "capacity_liter": cap, "batch_id": batchID,
			"batch_no": batchNo, "beer_style": style, "deposit_amount": dep, "status": status,
			"status_name": kegStatusNames[status], "location_type": locType, "location_id": locID,
			"location_name": locName, "opened_at": openedAt, "delivered_at": deliveredAt})
	}
	c.JSON(http.StatusOK, list)
}

func locationName(locType string, locID int) string {
	switch locType {
	case "warehouse":
		return "酒厂仓库"
	case "cleaning_plant":
		return "清洗厂"
	case "in_transit":
		return "运输途中"
	case "bar":
		var n string
		if db.QueryRow(`SELECT name FROM bars WHERE id=$1`, locID).Scan(&n) == nil {
			return n
		}
	case "distributor":
		var n string
		if db.QueryRow(`SELECT name FROM distributors WHERE id=$1`, locID).Scan(&n) == nil {
			return n
		}
	}
	return "-"
}

// createKeg 仓库入库灌装（新桶或清洗合格桶再灌装）
func createKeg(c *gin.Context) {
	var req struct {
		KegNo         string  `json:"keg_no"`
		CapacityLiter float64 `json:"capacity_liter"`
		BatchID       int     `json:"batch_id"`
		DepositAmount float64 `json:"deposit_amount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.KegNo == "" || req.BatchID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "桶编号和批次必填"})
		return
	}
	// 校验批次可售
	var bStatus string
	if err := db.QueryRow(`SELECT status FROM batches WHERE id=$1`, req.BatchID).Scan(&bStatus); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "批次不存在"})
		return
	}
	if bStatus != "normal" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该批次已停售/召回，禁止灌装发货"})
		return
	}
	if req.DepositAmount <= 0 {
		req.DepositAmount = 500
	}
	var id int
	err := db.QueryRow(`INSERT INTO kegs(keg_no,capacity_liter,batch_id,deposit_amount,status,location_type)
		VALUES($1,$2,$3,$4,'filled','warehouse') RETURNING id`,
		req.KegNo, req.CapacityLiter, req.BatchID, req.DepositAmount).Scan(&id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "创建失败：" + err.Error()})
		return
	}
	u := uid(c)
	addKegEvent(nil, id, "灌装入库", fmt.Sprintf("批次ID %d，押金 %.2f 元", req.BatchID, req.DepositAmount), &u)
	c.JSON(http.StatusOK, gin.H{"id": id})
}

// refillKeg 清洗合格桶再灌装
func refillKeg(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		BatchID int `json:"batch_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BatchID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择批次"})
		return
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM kegs WHERE id=$1`, id).Scan(&status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "酒桶不存在"})
		return
	}
	if status != "clean_passed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅清洗合格的酒桶具备再灌装资格"})
		return
	}
	var bStatus string
	if err := db.QueryRow(`SELECT status FROM batches WHERE id=$1`, req.BatchID).Scan(&bStatus); err != nil || bStatus != "normal" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "批次不存在或已停售/召回"})
		return
	}
	_, err := db.Exec(`UPDATE kegs SET batch_id=$1, status='filled', location_type='warehouse', location_id=NULL,
		opened_at=NULL, emptied_at=NULL, delivered_at=NULL WHERE id=$2`, req.BatchID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u := uid(c)
	addKegEvent(nil, id, "再灌装", fmt.Sprintf("清洗合格后重新灌装，批次ID %d", req.BatchID), &u)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// kegDetail 单桶全链路追溯
func kegDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var kegNo, status, locType string
	var cap, dep float64
	var batchID, locID, orderID *int
	var openedAt, emptiedAt, deliveredAt *string
	err := db.QueryRow(`SELECT keg_no,capacity_liter,batch_id,deposit_amount,status,location_type,location_id,
		current_order_id,opened_at::text,emptied_at::text,delivered_at::text
		FROM kegs WHERE id=$1`, id).Scan(&kegNo, &cap, &batchID, &dep, &status, &locType, &locID,
		&orderID, &openedAt, &emptiedAt, &deliveredAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "酒桶不存在"})
		return
	}
	batch := gin.H{}
	if batchID != nil {
		var no, style, bs string
		var bb string
		if db.QueryRow(`SELECT batch_no,beer_style,status,best_before FROM batches WHERE id=$1`, *batchID).
			Scan(&no, &style, &bs, &bb) == nil {
			batch = gin.H{"id": *batchID, "batch_no": no, "beer_style": style, "status": bs, "best_before": bb[:10]}
		}
	}
	// 当前所在酒吧/经销商
	locName := locationName(locType, 0)
	if locID != nil {
		locName = locationName(locType, *locID)
	}
	// 事件时间线
	evRows, _ := db.Query(`SELECT e.event,e.detail,coalesce(u.name,'系统'),e.created_at
		FROM keg_events e LEFT JOIN users u ON u.id=e.actor_id WHERE e.keg_id=$1 ORDER BY e.id`, id)
	events := []gin.H{}
	if evRows != nil {
		defer evRows.Close()
		for evRows.Next() {
			var ev, det, actor, ts string
			evRows.Scan(&ev, &det, &actor, &ts)
			events = append(events, gin.H{"event": ev, "detail": det, "actor": actor, "created_at": ts})
		}
	}
	// 反馈
	fbRows, _ := db.Query(`SELECT f.pour_speed,f.pressure_issue,f.taste_anomaly,f.complaint,f.created_at,b.name
		FROM feedbacks f JOIN bars b ON b.id=f.bar_id WHERE f.keg_id=$1 ORDER BY f.id`, id)
	fbs := []gin.H{}
	if fbRows != nil {
		defer fbRows.Close()
		for fbRows.Next() {
			var ps, pi, ta, ts, bn string
			var comp bool
			fbRows.Scan(&ps, &pi, &ta, &comp, &ts, &bn)
			fbs = append(fbs, gin.H{"pour_speed": ps, "pressure_issue": pi, "taste_anomaly": ta,
				"complaint": comp, "created_at": ts, "bar_name": bn})
		}
	}
	// 清洗记录
	clRows, _ := db.Query(`SELECT r.result,r.check_note,coalesce(u.name,''),r.created_at
		FROM cleaning_records r LEFT JOIN users u ON u.id=r.operator_id WHERE r.keg_id=$1 ORDER BY r.id`, id)
	cls := []gin.H{}
	if clRows != nil {
		defer clRows.Close()
		for clRows.Next() {
			var res, note, op, ts string
			clRows.Scan(&res, &note, &op, &ts)
			cls = append(cls, gin.H{"result": res, "check_note": note, "operator": op, "created_at": ts})
		}
	}
	// 押金流水
	depRows, _ := db.Query(`SELECT t.type,t.amount,t.reason,t.status,t.created_at
		FROM deposit_transactions t WHERE t.keg_id=$1 ORDER BY t.id`, id)
	deps := []gin.H{}
	if depRows != nil {
		defer depRows.Close()
		for depRows.Next() {
			var tp, reason, st, ts string
			var amt float64
			depRows.Scan(&tp, &amt, &reason, &st, &ts)
			deps = append(deps, gin.H{"type": tp, "amount": amt, "reason": reason, "status": st, "created_at": ts})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"id": id, "keg_no": kegNo, "capacity_liter": cap, "deposit_amount": dep,
		"status": status, "status_name": kegStatusNames[status],
		"location_type": locType, "location_id": locID, "location_name": locName,
		"batch": batch, "current_order_id": orderID,
		"opened_at": openedAt, "emptied_at": emptiedAt, "delivered_at": deliveredAt,
		"events": events, "feedbacks": fbs, "cleaning_records": cls, "deposit_transactions": deps,
	})
}
