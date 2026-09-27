package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ---------- 配送订单 ----------

func listOrders(c *gin.Context) {
	q := `SELECT o.id,o.order_no,o.from_type,coalesce(o.from_distributor_id,0),o.to_type,
		coalesce(o.to_distributor_id,0),coalesce(o.to_bar_id,0),coalesce(o.driver_id,0),
		o.status,o.is_holiday_temp,o.note,o.receiver_name,o.created_at,coalesce(o.delivered_at::text,'')
		FROM delivery_orders o WHERE 1=1`
	args := []interface{}{}
	n := 0
	if s := c.Query("status"); s != "" {
		n++
		q += fmt.Sprintf(" AND o.status=$%d", n)
		args = append(args, s)
	}
	if role(c) == "driver" {
		n++
		q += fmt.Sprintf(" AND o.driver_id=$%d", n)
		args = append(args, uid(c))
	}
	if role(c) == "bar_manager" {
		if org, ok := c.Get("org_id"); ok {
			n++
			q += fmt.Sprintf(" AND o.to_bar_id=$%d", n)
			args = append(args, org.(int))
		}
	}
	if role(c) == "distributor" {
		if org, ok := c.Get("org_id"); ok {
			n++
			q += fmt.Sprintf(" AND (o.from_distributor_id=$%d OR o.to_distributor_id=$%d)", n, n)
			args = append(args, org.(int))
		}
	}
	q += " ORDER BY o.id DESC LIMIT 300"
	rows, err := db.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, fromDist, toDist, toBar, driverID int
		var no, fromType, toType, status, note, receiver, createdAt, deliveredAt string
		var holiday bool
		rows.Scan(&id, &no, &fromType, &fromDist, &toType, &toDist, &toBar, &driverID,
			&status, &holiday, &note, &receiver, &createdAt, &deliveredAt)
		var kegCount int
		db.QueryRow(`SELECT count(*) FROM delivery_order_items WHERE order_id=$1`, id).Scan(&kegCount)
		toName := ""
		if toType == "bar" {
			db.QueryRow(`SELECT name FROM bars WHERE id=$1`, toBar).Scan(&toName)
		} else {
			db.QueryRow(`SELECT name FROM distributors WHERE id=$1`, toDist).Scan(&toName)
		}
		driverName := ""
		if driverID != 0 {
			db.QueryRow(`SELECT name FROM users WHERE id=$1`, driverID).Scan(&driverName)
		}
		list = append(list, gin.H{"id": id, "order_no": no, "from_type": fromType, "to_type": toType,
			"to_name": toName, "to_bar_id": toBar, "to_distributor_id": toDist,
			"driver_id": driverID, "driver_name": driverName, "status": status,
			"is_holiday_temp": holiday, "note": note, "receiver_name": receiver,
			"keg_count": kegCount, "created_at": createdAt, "delivered_at": deliveredAt})
	}
	c.JSON(http.StatusOK, list)
}

func orderDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var no, fromType, toType, status, note, receiver, createdAt string
	var fromDist, toDist, toBar, driverID *int
	var holiday bool
	var deliveredAt *string
	err := db.QueryRow(`SELECT order_no,from_type,from_distributor_id,to_type,to_distributor_id,to_bar_id,
		driver_id,status,is_holiday_temp,note,receiver_name,created_at,delivered_at::text
		FROM delivery_orders WHERE id=$1`, id).Scan(&no, &fromType, &fromDist, &toType, &toDist, &toBar,
		&driverID, &status, &holiday, &note, &receiver, &createdAt, &deliveredAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "订单不存在"})
		return
	}
	itemRows, _ := db.Query(`SELECT k.id,k.keg_no,k.capacity_liter,coalesce(b.batch_no,''),coalesce(b.beer_style,''),k.status
		FROM delivery_order_items i JOIN kegs k ON k.id=i.keg_id
		LEFT JOIN batches b ON b.id=k.batch_id WHERE i.order_id=$1 ORDER BY k.keg_no`, id)
	items := []gin.H{}
	if itemRows != nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var kid int
			var kegNo, batchNo, style, ks string
			var cap float64
			itemRows.Scan(&kid, &kegNo, &cap, &batchNo, &style, &ks)
			items = append(items, gin.H{"keg_id": kid, "keg_no": kegNo, "capacity_liter": cap,
				"batch_no": batchNo, "beer_style": style, "keg_status": ks, "keg_status_name": kegStatusNames[ks]})
		}
	}
	driverName := ""
	if driverID != nil {
		db.QueryRow(`SELECT name FROM users WHERE id=$1`, *driverID).Scan(&driverName)
	}
	toName := ""
	if toType == "bar" && toBar != nil {
		db.QueryRow(`SELECT name FROM bars WHERE id=$1`, *toBar).Scan(&toName)
	} else if toDist != nil {
		db.QueryRow(`SELECT name FROM distributors WHERE id=$1`, *toDist).Scan(&toName)
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "order_no": no, "from_type": fromType, "to_type": toType,
		"to_name": toName, "to_bar_id": toBar, "to_distributor_id": toDist,
		"driver_id": driverID, "driver_name": driverName, "status": status,
		"is_holiday_temp": holiday, "note": note, "receiver_name": receiver,
		"created_at": createdAt, "delivered_at": deliveredAt, "items": items})
}

// createOrder 创建配送订单（酒厂仓库或经销商发起）
func createOrder(c *gin.Context) {
	var req struct {
		ToType           string `json:"to_type"` // bar / distributor
		ToBarID          *int   `json:"to_bar_id"`
		ToDistributorID  *int   `json:"to_distributor_id"`
		DriverID         *int   `json:"driver_id"`
		KegIDs           []int  `json:"keg_ids"`
		IsHolidayTemp    bool   `json:"is_holiday_temp"`
		Note             string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.KegIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要配送的酒桶"})
		return
	}
	if req.ToType == "bar" && req.ToBarID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择目标酒吧"})
		return
	}
	if req.ToType == "distributor" && req.ToDistributorID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择目标经销商"})
		return
	}
	fromType := "brewery"
	var fromDist *int
	if role(c) == "distributor" {
		fromType = "distributor"
		if org, ok := c.Get("org_id"); ok {
			d := org.(int)
			fromDist = &d
		}
	}
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	// 校验每个桶可用且批次可售
	for _, kid := range req.KegIDs {
		var status, locType string
		var locID *int
		var batchStatus string
		err := tx.QueryRow(`SELECT k.status,k.location_type,k.location_id,coalesce(b.status,'normal')
			FROM kegs k LEFT JOIN batches b ON b.id=k.batch_id WHERE k.id=$1 FOR UPDATE OF k`,
			kid).Scan(&status, &locType, &locID, &batchStatus)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("酒桶 %d 不存在", kid)})
			return
		}
		if batchStatus != "normal" {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("酒桶 %d 所属批次已停售/召回，禁止发货", kid)})
			return
		}
		if fromType == "brewery" && status != "filled" {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("酒桶 %d 不在酒厂可发库存中（当前：%s）", kid, kegStatusNames[status])})
			return
		}
		if fromType == "distributor" && (status != "at_distributor" || locID == nil || *locID != *fromDist) {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("酒桶 %d 不在本经销商库存中", kid)})
			return
		}
	}
	var orderID int
	var orderNo string
	tx.QueryRow(`SELECT 'DO-'||to_char(now(),'YYYYMMDD')||'-'||lpad((count(*)+1)::text,4,'0') FROM delivery_orders`).Scan(&orderNo)
	err = tx.QueryRow(`INSERT INTO delivery_orders(order_no,from_type,from_distributor_id,to_type,to_distributor_id,to_bar_id,
		driver_id,status,is_holiday_temp,note,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10) RETURNING id`,
		orderNo, fromType, fromDist, req.ToType, req.ToDistributorID, req.ToBarID,
		req.DriverID, req.IsHolidayTemp, req.Note, uid(c)).Scan(&orderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u := uid(c)
	for _, kid := range req.KegIDs {
		if _, err := tx.Exec(`INSERT INTO delivery_order_items(order_id,keg_id) VALUES($1,$2)`, orderID, kid); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "酒桶重复或无效"})
			return
		}
		if _, err := tx.Exec(`UPDATE kegs SET status='in_transit', location_type='in_transit', location_id=NULL,
			current_order_id=$1 WHERE id=$2`, orderID, kid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		addKegEvent(tx, kid, "出库配送", "订单 "+orderNo, &u)
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.DriverID != nil {
		notify(req.DriverID, "", nil, "order", "新配送任务 "+orderNo, "请及时配送并确认收货人。", "order", &orderID)
	}
	c.JSON(http.StatusOK, gin.H{"id": orderID, "order_no": orderNo})
}

// dispatchOrder 司机开始配送
func dispatchOrder(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	res, err := db.Exec(`UPDATE delivery_orders SET status='delivering' WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订单状态不允许发货"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// deliverOrder 确认送达：记录收货人，桶位置转移，押金挂账
func deliverOrder(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		ReceiverName string `json:"receiver_name"`
	}
	c.ShouldBindJSON(&req)
	if req.ReceiverName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写收货人"})
		return
	}
	var status, toType string
	var toBar, toDist *int
	var orderNo string
	err := db.QueryRow(`SELECT status,to_type,to_bar_id,to_distributor_id,order_no FROM delivery_orders WHERE id=$1`,
		id).Scan(&status, &toType, &toBar, &toDist, &orderNo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "订单不存在"})
		return
	}
	if status != "pending" && status != "delivering" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订单已送达或已取消"})
		return
	}
	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE delivery_orders SET status='delivered', receiver_name=$1, delivered_at=now() WHERE id=$2`,
		req.ReceiverName, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 确定押金账户与账期
	ownerType := "bar"
	ownerID := 0
	termDays := 30
	if toType == "bar" && toBar != nil {
		ownerID = *toBar
		tx.QueryRow(`SELECT deposit_credit_term_days FROM bars WHERE id=$1`, ownerID).Scan(&termDays)
	} else if toType == "distributor" && toDist != nil {
		ownerType = "distributor"
		ownerID = *toDist
	}
	accountID, err := ensureDepositAccount(tx, ownerType, ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	kegRows, err := tx.Query(`SELECT k.id,k.keg_no,k.deposit_amount FROM delivery_order_items i
		JOIN kegs k ON k.id=i.keg_id WHERE i.order_id=$1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type kinfo struct {
		id  int
		no  string
		dep float64
	}
	var kegs []kinfo
	for kegRows.Next() {
		var k kinfo
		kegRows.Scan(&k.id, &k.no, &k.dep)
		kegs = append(kegs, k)
	}
	kegRows.Close()
	u := uid(c)
	newStatus := "delivered"
	newLocType := "bar"
	var newLocID *int = toBar
	if toType == "distributor" {
		newStatus = "at_distributor"
		newLocType = "distributor"
		newLocID = toDist
	}
	for _, k := range kegs {
		if _, err := tx.Exec(`UPDATE kegs SET status=$1, location_type=$2, location_id=$3, delivered_at=now()
			WHERE id=$4`, newStatus, newLocType, newLocID, k.id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// 押金挂账（账期内 pending）
		if _, err := tx.Exec(`INSERT INTO deposit_transactions(account_id,keg_id,type,amount,reason,ref_type,ref_id,status,due_date,created_by)
			VALUES($1,$2,'hold',$3,$4,'order',$5,'pending',(CURRENT_DATE + $6::int * INTERVAL '1 day')::date,$7)`,
			accountID, k.id, k.dep, "配送收货押金挂账（订单 "+orderNo+"）", id, termDays, u); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if _, err := tx.Exec(`UPDATE deposit_accounts SET balance=balance+$1, updated_at=now() WHERE id=$2`,
			k.dep, accountID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		addKegEvent(tx, k.id, "送达签收", "收货人："+req.ReceiverName+"（订单 "+orderNo+"）", &u)
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if toType == "bar" && toBar != nil {
		notify(nil, "", toBar, "order", "订单 "+orderNo+" 已送达", "收货人："+req.ReceiverName, "order", &id)
	}
	notifyRole("finance", "deposit", "订单 "+orderNo+" 押金挂账", fmt.Sprintf("共 %d 桶，收货人 %s", len(kegs), req.ReceiverName), "order", &id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// cancelOrder 取消订单：桶回退到发货方库存
func cancelOrder(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var status, fromType string
	var fromDist *int
	err := db.QueryRow(`SELECT status,from_type,from_distributor_id FROM delivery_orders WHERE id=$1`,
		id).Scan(&status, &fromType, &fromDist)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "订单不存在"})
		return
	}
	if status == "delivered" || status == "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "订单已送达或已取消"})
		return
	}
	tx, _ := db.Begin()
	defer tx.Rollback()
	tx.Exec(`UPDATE delivery_orders SET status='cancelled' WHERE id=$1`, id)
	backStatus, backType := "filled", "warehouse"
	var backLoc *int
	if fromType == "distributor" {
		backStatus, backType = "at_distributor", "distributor"
		backLoc = fromDist
	}
	rows, _ := tx.Query(`SELECT keg_id FROM delivery_order_items WHERE order_id=$1`, id)
	var kids []int
	if rows != nil {
		for rows.Next() {
			var k int
			rows.Scan(&k)
			kids = append(kids, k)
		}
		rows.Close()
	}
	u := uid(c)
	for _, k := range kids {
		tx.Exec(`UPDATE kegs SET status=$1, location_type=$2, location_id=$3, current_order_id=NULL WHERE id=$4`,
			backStatus, backType, backLoc, k)
		addKegEvent(tx, k, "订单取消退回", "", &u)
	}
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 酒吧端操作 ----------

func barIDOf(c *gin.Context) (int, bool) {
	if role(c) == "bar_manager" {
		if org, ok := c.Get("org_id"); ok {
			return org.(int), true
		}
	}
	return 0, false
}

// openKeg 酒吧开桶
func openKeg(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	barID, _ := barIDOf(c)
	var status, locType string
	var locID *int
	var batchStatus string
	var kegNo string
	err := db.QueryRow(`SELECT k.status,k.location_type,k.location_id,coalesce(b.status,'normal'),k.keg_no
		FROM kegs k LEFT JOIN batches b ON b.id=k.batch_id WHERE k.id=$1`, id).
		Scan(&status, &locType, &locID, &batchStatus, &kegNo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "酒桶不存在"})
		return
	}
	if role(c) == "bar_manager" && (locType != "bar" || locID == nil || *locID != barID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "该酒桶不在本酒吧"})
		return
	}
	if status != "delivered" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅未开桶的酒桶可以开桶（当前："+kegStatusNames[status]+"）"})
		return
	}
	if batchStatus != "normal" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该批次已停售/召回，禁止开桶，请等待召回通知"})
		return
	}
	u := uid(c)
	db.Exec(`UPDATE kegs SET status='opened', opened_at=now() WHERE id=$1`, id)
	addKegEvent(nil, id, "开桶", "", &u)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// emptyKeg 酒吧确认打空
func emptyKeg(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	barID, _ := barIDOf(c)
	var status, locType string
	var locID *int
	err := db.QueryRow(`SELECT status,location_type,location_id FROM kegs WHERE id=$1`, id).
		Scan(&status, &locType, &locID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "酒桶不存在"})
		return
	}
	if role(c) == "bar_manager" && (locType != "bar" || locID == nil || *locID != barID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "该酒桶不在本酒吧"})
		return
	}
	if status != "opened" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅已开桶的酒桶可确认打空"})
		return
	}
	u := uid(c)
	db.Exec(`UPDATE kegs SET status='empty', emptied_at=now() WHERE id=$1`, id)
	addKegEvent(nil, id, "打空", "", &u)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// createFeedback 开桶反馈
func createFeedback(c *gin.Context) {
	var req struct {
		KegID         int    `json:"keg_id"`
		PourSpeed     string `json:"pour_speed"`
		PressureIssue string `json:"pressure_issue"`
		TasteAnomaly  string `json:"taste_anomaly"`
		TasteNote     string `json:"taste_note"`
		Complaint     bool   `json:"complaint"`
		ComplaintNote string `json:"complaint_note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.KegID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	barID, _ := barIDOf(c)
	var locType, kegNo string
	var locID, batchID *int
	err := db.QueryRow(`SELECT location_type,location_id,batch_id,keg_no FROM kegs WHERE id=$1`, req.KegID).
		Scan(&locType, &locID, &batchID, &kegNo)
	if err != nil || batchID == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "酒桶不存在或未关联批次"})
		return
	}
	if role(c) == "bar_manager" && (locType != "bar" || locID == nil || *locID != barID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "该酒桶不在本酒吧"})
		return
	}
	if req.PourSpeed == "" {
		req.PourSpeed = "正常"
	}
	if req.PressureIssue == "" {
		req.PressureIssue = "正常"
	}
	if req.TasteAnomaly == "" {
		req.TasteAnomaly = "无"
	}
	u := uid(c)
	var fid int
	err = db.QueryRow(`INSERT INTO feedbacks(keg_id,bar_id,batch_id,pour_speed,pressure_issue,taste_anomaly,taste_note,complaint,complaint_note,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		req.KegID, barID, *batchID, req.PourSpeed, req.PressureIssue, req.TasteAnomaly,
		req.TasteNote, req.Complaint, req.ComplaintNote, u).Scan(&fid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	addKegEvent(nil, req.KegID, "开桶反馈",
		fmt.Sprintf("出酒:%s 气压:%s 口味:%s 客诉:%v", req.PourSpeed, req.PressureIssue, req.TasteAnomaly, req.Complaint), &u)
	// 严重反馈即时通知品控
	if req.TasteAnomaly == "明显" || req.TasteAnomaly == "严重" || req.Complaint {
		notifyRole("qc", "incident", "酒桶 "+kegNo+" 收到严重反馈",
			fmt.Sprintf("口味异常：%s；客诉：%v。%s", req.TasteAnomaly, req.Complaint, req.ComplaintNote), "keg", &req.KegID)
	}
	checkBatchComplaints(*batchID)
	c.JSON(http.StatusOK, gin.H{"id": fid})
}

// ---------- 回收预约与司机扫码 ----------

func listAppointments(c *gin.Context) {
	q := `SELECT a.id,a.appt_no,a.bar_id,b.name,a.status,a.scheduled_date::text,coalesce(a.driver_id,0),
		a.note,a.created_at,coalesce(a.completed_at::text,'')
		FROM pickup_appointments a JOIN bars b ON b.id=a.bar_id WHERE 1=1`
	args := []interface{}{}
	n := 0
	if role(c) == "bar_manager" {
		if org, ok := c.Get("org_id"); ok {
			n++
			q += fmt.Sprintf(" AND a.bar_id=$%d", n)
			args = append(args, org.(int))
		}
	}
	if role(c) == "driver" {
		n++
		q += fmt.Sprintf(" AND (a.driver_id=$%d OR a.driver_id IS NULL)", n)
		args = append(args, uid(c))
	}
	if s := c.Query("status"); s != "" {
		n++
		q += fmt.Sprintf(" AND a.status=$%d", n)
		args = append(args, s)
	}
	q += " ORDER BY a.id DESC LIMIT 200"
	rows, err := db.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, barID, driverID int
		var no, barName, status, date, note, createdAt, completedAt string
		rows.Scan(&id, &no, &barID, &barName, &status, &date, &driverID, &note, &createdAt, &completedAt)
		var scanCount int
		db.QueryRow(`SELECT count(*) FROM pickup_scans WHERE appointment_id=$1`, id).Scan(&scanCount)
		driverName := ""
		if driverID != 0 {
			db.QueryRow(`SELECT name FROM users WHERE id=$1`, driverID).Scan(&driverName)
		}
		list = append(list, gin.H{"id": id, "appt_no": no, "bar_id": barID, "bar_name": barName,
			"status": status, "scheduled_date": date[:10], "driver_id": driverID, "driver_name": driverName,
			"note": note, "scan_count": scanCount, "created_at": createdAt, "completed_at": completedAt})
	}
	c.JSON(http.StatusOK, list)
}

// createAppointment 酒吧预约回收
func createAppointment(c *gin.Context) {
	var req struct {
		ScheduledDate string `json:"scheduled_date"`
		Note          string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ScheduledDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择预约日期"})
		return
	}
	barID, ok := barIDOf(c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅酒吧店长可预约回收"})
		return
	}
	var emptyCount int
	db.QueryRow(`SELECT count(*) FROM kegs WHERE location_type='bar' AND location_id=$1 AND status='empty'`,
		barID).Scan(&emptyCount)
	if emptyCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前没有已打空待回收的酒桶"})
		return
	}
	var id int
	var no string
	db.QueryRow(`SELECT 'PK-'||to_char(now(),'YYYYMMDD')||'-'||lpad((count(*)+1)::text,4,'0') FROM pickup_appointments`).Scan(&no)
	u := uid(c)
	err := db.QueryRow(`INSERT INTO pickup_appointments(appt_no,bar_id,scheduled_date,note,created_by)
		VALUES($1,$2,$3,$4,$5) RETURNING id`, no, barID, req.ScheduledDate, req.Note, u).Scan(&id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 将该酒吧所有 empty 桶标记为已预约
	db.Exec(`UPDATE kegs SET status='pickup_scheduled' WHERE location_type='bar' AND location_id=$1 AND status='empty'`, barID)
	notifyRole("driver", "pickup", "新回收预约 "+no, fmt.Sprintf("预约日期 %s，约 %d 只空桶", req.ScheduledDate, emptyCount), "appointment", &id)
	notifyRole("warehouse", "pickup", "新回收预约 "+no, fmt.Sprintf("酒吧ID %d，%d 只空桶待回收", barID, emptyCount), "appointment", &id)
	c.JSON(http.StatusOK, gin.H{"id": id, "appt_no": no, "keg_count": emptyCount})
}

func appointmentDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var no, status, date, note, createdAt string
	var barID int
	var driverID *int
	var completedAt *string
	err := db.QueryRow(`SELECT appt_no,bar_id,status,scheduled_date::text,driver_id,note,created_at,completed_at::text
		FROM pickup_appointments WHERE id=$1`, id).
		Scan(&no, &barID, &status, &date, &driverID, &note, &createdAt, &completedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "预约不存在"})
		return
	}
	var barName string
	db.QueryRow(`SELECT name FROM bars WHERE id=$1`, barID).Scan(&barName)
	// 该酒吧待回收桶（empty / pickup_scheduled）
	kegRows, _ := db.Query(`SELECT k.id,k.keg_no,k.status,coalesce(b.batch_no,''),
		(SELECT count(*) FROM pickup_scans s WHERE s.appointment_id=$1 AND s.keg_id=k.id) AS scanned
		FROM kegs k LEFT JOIN batches b ON b.id=k.batch_id
		WHERE k.location_type='bar' AND k.location_id=$2 AND k.status IN ('empty','pickup_scheduled')
		ORDER BY k.keg_no`, id, barID)
	kegs := []gin.H{}
	if kegRows != nil {
		defer kegRows.Close()
		for kegRows.Next() {
			var kid, scanned int
			var kegNo, ks, batchNo string
			kegRows.Scan(&kid, &kegNo, &ks, &batchNo, &scanned)
			kegs = append(kegs, gin.H{"keg_id": kid, "keg_no": kegNo, "status": ks,
				"status_name": kegStatusNames[ks], "batch_no": batchNo, "scanned": scanned > 0})
		}
	}
	scanRows, _ := db.Query(`SELECT s.keg_id,k.keg_no,s.appearance,s.residual,s.cap_status,s.scanned_at,coalesce(u.name,'')
		FROM pickup_scans s JOIN kegs k ON k.id=s.keg_id LEFT JOIN users u ON u.id=s.scanned_by
		WHERE s.appointment_id=$1 ORDER BY s.id`, id)
	scans := []gin.H{}
	if scanRows != nil {
		defer scanRows.Close()
		for scanRows.Next() {
			var kid int
			var kegNo, app, res, capS, ts, driver string
			scanRows.Scan(&kid, &kegNo, &app, &res, &capS, &ts, &driver)
			scans = append(scans, gin.H{"keg_id": kid, "keg_no": kegNo, "appearance": app,
				"residual": res, "cap_status": capS, "scanned_at": ts, "driver": driver})
		}
	}
	driverName := ""
	if driverID != nil {
		db.QueryRow(`SELECT name FROM users WHERE id=$1`, *driverID).Scan(&driverName)
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "appt_no": no, "bar_id": barID, "bar_name": barName,
		"status": status, "scheduled_date": date[:10], "driver_id": driverID, "driver_name": driverName,
		"note": note, "created_at": createdAt, "completed_at": completedAt,
		"pending_kegs": kegs, "scans": scans})
}

// claimAppointment 司机接单
func claimAppointment(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	u := uid(c)
	res, err := db.Exec(`UPDATE pickup_appointments SET driver_id=$1, status='scanning'
		WHERE id=$2 AND status IN ('scheduled','scanning') AND (driver_id IS NULL OR driver_id=$1)`, u, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预约已被其他司机接走或已完成"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// scanKeg 司机扫码确认空桶
func scanKeg(c *gin.Context) {
	apptID, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		KegNo      string `json:"keg_no"` // 扫码得到桶编号
		Appearance string `json:"appearance"`
		Residual   string `json:"residual"`
		CapStatus  string `json:"cap_status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.KegNo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请扫描桶编号"})
		return
	}
	var barID int
	var status string
	err := db.QueryRow(`SELECT bar_id,status FROM pickup_appointments WHERE id=$1`, apptID).Scan(&barID, &status)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "预约不存在"})
		return
	}
	if status == "completed" || status == "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预约已结束"})
		return
	}
	var kegID, locID int
	var kegStatus, locType string
	err = db.QueryRow(`SELECT id,status,location_type,coalesce(location_id,0) FROM kegs WHERE keg_no=$1`,
		req.KegNo).Scan(&kegID, &kegStatus, &locType, &locID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "桶编号不存在：" + req.KegNo})
		return
	}
	if locType != "bar" || locID != barID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该桶不在预约酒吧，无法回收"})
		return
	}
	if kegStatus != "empty" && kegStatus != "pickup_scheduled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该桶当前状态（"+kegStatusNames[kegStatus]+"）不可回收"})
		return
	}
	if req.Appearance == "" {
		req.Appearance = "完好"
	}
	if req.Residual == "" {
		req.Residual = "无残液"
	}
	if req.CapStatus == "" {
		req.CapStatus = "封盖完好"
	}
	u := uid(c)
	tx, _ := db.Begin()
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO pickup_scans(appointment_id,keg_id,scanned_by,appearance,residual,cap_status)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(appointment_id,keg_id) DO UPDATE
		SET appearance=EXCLUDED.appearance,residual=EXCLUDED.residual,cap_status=EXCLUDED.cap_status,scanned_at=now()`,
		apptID, kegID, u, req.Appearance, req.Residual, req.CapStatus)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tx.Exec(`UPDATE pickup_appointments SET status='scanning', driver_id=$1 WHERE id=$2 AND status='scheduled'`, u, apptID)
	// 桶进入已回收待清洗
	tx.Exec(`UPDATE kegs SET status='returned', location_type='cleaning_plant', location_id=NULL WHERE id=$1`, kegID)
	addKegEvent(tx, kegID, "回收扫码",
		fmt.Sprintf("外观:%s 残液:%s 封盖:%s", req.Appearance, req.Residual, req.CapStatus), &u)
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 桶体变形 → 异常事件
	if req.Appearance == "桶体变形" {
		title := "酒桶 " + req.KegNo + " 桶体变形"
		iid, err := createIncident("deformed", &kegID, nil, &barID, title,
			"司机回收扫码发现桶体变形，请仓库与财务评估押金扣款与报废处理。")
		if err == nil {
			addKegEvent(nil, kegID, "异常上报", title, &u)
			_ = iid
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "keg_id": kegID})
}

// completeAppointment 司机完成回收：漏扫检测
func completeAppointment(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var barID, driverID int
	var status, apptNo string
	err := db.QueryRow(`SELECT bar_id,coalesce(driver_id,0),status,appt_no FROM pickup_appointments WHERE id=$1`,
		id).Scan(&barID, &driverID, &status, &apptNo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "预约不存在"})
		return
	}
	if status == "completed" || status == "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "预约已结束"})
		return
	}
	u := uid(c)
	if driverID == 0 {
		db.Exec(`UPDATE pickup_appointments SET driver_id=$1 WHERE id=$2`, u, id)
		driverID = u
	}
	// 漏扫检测：该酒吧仍处于 empty/pickup_scheduled 且本预约未扫码的桶
	missedRows, _ := db.Query(`SELECT k.id,k.keg_no FROM kegs k
		WHERE k.location_type='bar' AND k.location_id=$1 AND k.status IN ('empty','pickup_scheduled')
		  AND NOT EXISTS (SELECT 1 FROM pickup_scans s WHERE s.appointment_id=$2 AND s.keg_id=k.id)`, barID, id)
	type missed struct {
		id int
		no string
	}
	var missedList []missed
	if missedRows != nil {
		for missedRows.Next() {
			var m missed
			missedRows.Scan(&m.id, &m.no)
			missedList = append(missedList, m)
		}
		missedRows.Close()
	}
	db.Exec(`UPDATE pickup_appointments SET status='completed', completed_at=now() WHERE id=$1`, id)
	// 漏扫 → 异常事件（司机责任）
	if len(missedList) > 0 {
		nos := ""
		for i, m := range missedList {
			if i > 0 {
				nos += "、"
			}
			nos += m.no
		}
		title := fmt.Sprintf("回收预约 %s 漏扫 %d 只空桶", apptNo, len(missedList))
		desc := "司机完成回收时仍有空桶未扫码：" + nos + "。请司机补扫或说明，仓库跟进。"
		iid, err := createIncident("missed_scan", nil, nil, &barID, title, desc)
		if err == nil && driverID != 0 {
			d := driverID
			notify(&d, "", nil, "incident", title, desc, "incident", &iid)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "missed_count": len(missedList)})
}

// reportLost 上报酒桶丢失（酒吧/司机/仓库）
func reportLost(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var kegNo, status string
	var locID *int
	var locType string
	err := db.QueryRow(`SELECT keg_no,status,location_type,location_id FROM kegs WHERE id=$1`, id).
		Scan(&kegNo, &status, &locType, &locID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "酒桶不存在"})
		return
	}
	if status == "lost" || status == "scrapped" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "酒桶已登记丢失/报废"})
		return
	}
	u := uid(c)
	db.Exec(`UPDATE kegs SET status='lost' WHERE id=$1`, id)
	addKegEvent(nil, id, "登记丢失", "", &u)
	var barID *int
	if locType == "bar" {
		barID = locID
	}
	title := "酒桶 " + kegNo + " 丢失"
	iid, _ := createIncident("lost", &id, nil, barID, title,
		"酒桶登记丢失，请财务评估押金扣款，仓库核减库存。")
	if barID != nil {
		notify(nil, "", barID, "incident", title, "如找回请联系仓库撤销。", "incident", &iid)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "incident_id": iid})
}

// ---------- 清洗厂 ----------

// listCleaningQueue 待清洗桶
func listCleaningQueue(c *gin.Context) {
	rows, err := db.Query(`SELECT k.id,k.keg_no,k.capacity_liter,coalesce(b.batch_no,''),
		(SELECT s.appearance FROM pickup_scans s WHERE s.keg_id=k.id ORDER BY s.id DESC LIMIT 1),
		(SELECT s.residual FROM pickup_scans s WHERE s.keg_id=k.id ORDER BY s.id DESC LIMIT 1),
		(SELECT s.cap_status FROM pickup_scans s WHERE s.keg_id=k.id ORDER BY s.id DESC LIMIT 1)
		FROM kegs k LEFT JOIN batches b ON b.id=k.batch_id
		WHERE k.status='returned' ORDER BY k.id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id int
		var kegNo, batchNo string
		var app, res, capS sql.NullString
		var cap float64
		rows.Scan(&id, &kegNo, &cap, &batchNo, &app, &res, &capS)
		list = append(list, gin.H{"id": id, "keg_no": kegNo, "capacity_liter": cap, "batch_no": batchNo,
			"appearance": app.String, "residual": res.String, "cap_status": capS.String})
	}
	c.JSON(http.StatusOK, list)
}

// recordCleaning 清洗检测登记
func recordCleaning(c *gin.Context) {
	var req struct {
		KegID     int    `json:"keg_id"`
		Result    string `json:"result"` // passed / failed
		CheckNote string `json:"check_note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Result != "passed" && req.Result != "failed") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "检测结果必填（passed/failed）"})
		return
	}
	var status, kegNo string
	var batchID *int
	err := db.QueryRow(`SELECT status,keg_no,batch_id FROM kegs WHERE id=$1`, req.KegID).
		Scan(&status, &kegNo, &batchID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "酒桶不存在"})
		return
	}
	if status != "returned" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该桶不在待清洗状态（当前："+kegStatusNames[status]+"）"})
		return
	}
	u := uid(c)
	tx, _ := db.Begin()
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO cleaning_records(keg_id,operator_id,result,check_note) VALUES($1,$2,$3,$4)`,
		req.KegID, u, req.Result, req.CheckNote); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	newStatus := "clean_passed"
	if req.Result == "failed" {
		newStatus = "clean_failed"
	}
	tx.Exec(`UPDATE kegs SET status=$1 WHERE id=$2`, newStatus, req.KegID)
	resultName := "清洗合格"
	if req.Result == "failed" {
		resultName = "清洗不合格"
	}
	addKegEvent(tx, req.KegID, "清洗检测", resultName+"。"+req.CheckNote, &u)
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.Result == "failed" {
		title := "酒桶 " + kegNo + " 清洗检测不合格"
		createIncident("cleaning_failed", &req.KegID, batchID, nil, title,
			"清洗厂检测不合格："+req.CheckNote+"。请评估重新清洗或报废，并确认是否影响再灌装资格。")
	} else {
		// 清洗合格 → 押金可释放提示财务
		notifyRole("finance", "deposit", "酒桶 "+kegNo+" 清洗合格", "可释放对应押金挂账。", "keg", &req.KegID)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
