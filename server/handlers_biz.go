package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ---------- 押金财务 ----------

func listDepositAccounts(c *gin.Context) {
	rows, err := db.Query(`SELECT a.id,a.owner_type,a.owner_id,a.balance,
		CASE a.owner_type WHEN 'bar' THEN (SELECT name FROM bars WHERE id=a.owner_id)
			ELSE (SELECT name FROM distributors WHERE id=a.owner_id) END,
		CASE a.owner_type WHEN 'bar' THEN (SELECT deposit_credit_term_days FROM bars WHERE id=a.owner_id) ELSE 30 END
		FROM deposit_accounts a ORDER BY a.owner_type, a.owner_id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, ownerID, term int
		var ownerType, name string
		var bal float64
		rows.Scan(&id, &ownerType, &ownerID, &bal, &name, &term)
		var pending, overdue int
		var pendingAmt float64
		db.QueryRow(`SELECT count(*), coalesce(sum(amount),0) FROM deposit_transactions
			WHERE account_id=$1 AND status='pending'`, id).Scan(&pending, &pendingAmt)
		db.QueryRow(`SELECT count(*) FROM deposit_transactions
			WHERE account_id=$1 AND status='pending' AND due_date < CURRENT_DATE`, id).Scan(&overdue)
		list = append(list, gin.H{"id": id, "owner_type": ownerType, "owner_id": ownerID,
			"owner_name": name, "balance": bal, "credit_term_days": term,
			"pending_count": pending, "pending_amount": pendingAmt, "overdue_count": overdue})
	}
	c.JSON(http.StatusOK, list)
}

func listDepositTransactions(c *gin.Context) {
	accountID, _ := strconv.Atoi(c.Param("id"))
	rows, err := db.Query(`SELECT t.id,t.keg_id,coalesce(k.keg_no,''),t.type,t.amount,t.reason,t.ref_type,
		coalesce(t.ref_id,0),t.status,coalesce(t.due_date::text,''),t.created_at,coalesce(u.name,'')
		FROM deposit_transactions t LEFT JOIN kegs k ON k.id=t.keg_id LEFT JOIN users u ON u.id=t.created_by
		WHERE t.account_id=$1 ORDER BY t.id DESC LIMIT 300`, accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, refID int
		var kegID *int
		var kegNo, tp, reason, refType, status, due, createdAt, creator string
		var amt float64
		rows.Scan(&id, &kegID, &kegNo, &tp, &amt, &reason, &refType, &refID, &status, &due, &createdAt, &creator)
		overdue := status == "pending" && due != "" && due[:10] < today()
		list = append(list, gin.H{"id": id, "keg_id": kegID, "keg_no": kegNo, "type": tp, "amount": amt,
			"reason": reason, "ref_type": refType, "ref_id": refID, "status": status,
			"due_date": due, "overdue": overdue, "created_at": createdAt, "created_by": creator})
	}
	c.JSON(http.StatusOK, list)
}

func today() string {
	var t string
	db.QueryRow(`SELECT CURRENT_DATE::text`).Scan(&t)
	return t
}

// settleDeposit 财务按桶释放/扣减押金
func settleDeposit(c *gin.Context) {
	var req struct {
		KegID  int     `json:"keg_id"`
		Action string  `json:"action"` // release / deduct
		Amount float64 `json:"amount"` // deduct 时必填（可部分扣）
		Reason string  `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.KegID == 0 ||
		(req.Action != "release" && req.Action != "deduct") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	// 找到该桶未结算的 hold 流水
	var txID, accountID int
	var holdAmt float64
	err := db.QueryRow(`SELECT t.id,t.account_id,t.amount FROM deposit_transactions t
		WHERE t.keg_id=$1 AND t.type='hold' AND t.status='pending'
		ORDER BY t.id DESC LIMIT 1`, req.KegID).Scan(&txID, &accountID, &holdAmt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该桶没有待结算的押金挂账"})
		return
	}
	u := uid(c)
	tx, _ := db.Begin()
	defer tx.Rollback()
	if req.Action == "release" {
		// 释放：hold 结算 + 生成 release 流水，余额减少
		if _, err := tx.Exec(`UPDATE deposit_transactions SET status='settled' WHERE id=$1`, txID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if _, err := tx.Exec(`INSERT INTO deposit_transactions(account_id,keg_id,type,amount,reason,ref_type,status,created_by)
			VALUES($1,$2,'release',$3,$4,'manual','settled',$5)`,
			accountID, req.KegID, holdAmt, coalesce(req.Reason, "空桶回收合格，押金释放"), u); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		tx.Exec(`UPDATE deposit_accounts SET balance=balance-$1, updated_at=now() WHERE id=$2`, holdAmt, accountID)
	} else {
		if req.Amount <= 0 || req.Amount > holdAmt {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("扣款金额需在 0 ~ %.2f 之间", holdAmt)})
			return
		}
		if _, err := tx.Exec(`UPDATE deposit_transactions SET status='settled' WHERE id=$1`, txID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// 扣款流水 + 剩余部分释放
		if _, err := tx.Exec(`INSERT INTO deposit_transactions(account_id,keg_id,type,amount,reason,ref_type,status,created_by)
			VALUES($1,$2,'deduct',$3,$4,'manual','settled',$5)`,
			accountID, req.KegID, req.Amount, coalesce(req.Reason, "异常扣款"), u); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if remain := holdAmt - req.Amount; remain > 0 {
			tx.Exec(`INSERT INTO deposit_transactions(account_id,keg_id,type,amount,reason,ref_type,status,created_by)
				VALUES($1,$2,'release',$3,'扣除后剩余押金释放','manual','settled',$4)`,
				accountID, req.KegID, remain, u)
		}
		tx.Exec(`UPDATE deposit_accounts SET balance=balance-$1, updated_at=now() WHERE id=$2`, holdAmt, accountID)
	}
	var kegNo string
	tx.QueryRow(`SELECT keg_no FROM kegs WHERE id=$1`, req.KegID).Scan(&kegNo)
	actionName := "押金释放"
	if req.Action == "deduct" {
		actionName = fmt.Sprintf("押金扣款 %.2f 元", req.Amount)
	}
	addKegEvent(tx, req.KegID, actionName, req.Reason, &u)
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 通知账户归属方
	var ownerType string
	var ownerID int
	db.QueryRow(`SELECT owner_type,owner_id FROM deposit_accounts WHERE id=$1`, accountID).Scan(&ownerType, &ownerID)
	if ownerType == "bar" {
		notify(nil, "", &ownerID, "deposit", "酒桶 "+kegNo+" "+actionName, req.Reason, "keg", &req.KegID)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func coalesce(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---------- 异常事件（多方协同） ----------

func listIncidents(c *gin.Context) {
	q := `SELECT i.id,i.incident_no,i.type,coalesce(i.keg_id,0),coalesce(k.keg_no,''),
		coalesce(i.batch_id,0),coalesce(b.batch_no,''),coalesce(i.bar_id,0),coalesce(br.name,''),
		i.title,i.status,i.created_at
		FROM incidents i
		LEFT JOIN kegs k ON k.id=i.keg_id
		LEFT JOIN batches b ON b.id=i.batch_id
		LEFT JOIN bars br ON br.id=i.bar_id WHERE 1=1`
	args := []interface{}{}
	n := 0
	if s := c.Query("status"); s != "" {
		n++
		q += fmt.Sprintf(" AND i.status=$%d", n)
		args = append(args, s)
	}
	if s := c.Query("type"); s != "" {
		n++
		q += fmt.Sprintf(" AND i.type=$%d", n)
		args = append(args, s)
	}
	q += " ORDER BY i.id DESC LIMIT 200"
	rows, err := db.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, kegID, batchID, barID int
		var no, tp, kegNo, batchNo, barName, title, status, createdAt string
		rows.Scan(&id, &no, &tp, &kegID, &kegNo, &batchID, &batchNo, &barID, &barName, &title, &status, &createdAt)
		var actionCount int
		db.QueryRow(`SELECT count(*) FROM incident_actions WHERE incident_id=$1`, id).Scan(&actionCount)
		list = append(list, gin.H{"id": id, "incident_no": no, "type": tp, "keg_id": kegID, "keg_no": kegNo,
			"batch_id": batchID, "batch_no": batchNo, "bar_id": barID, "bar_name": barName,
			"title": title, "status": status, "action_count": actionCount, "created_at": createdAt})
	}
	c.JSON(http.StatusOK, list)
}

func incidentDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var no, tp, title, desc, status, resolution, createdAt string
	var kegID, batchID, barID *int
	var resolvedAt *string
	err := db.QueryRow(`SELECT incident_no,type,keg_id,batch_id,bar_id,title,description,status,resolution,created_at,resolved_at::text
		FROM incidents WHERE id=$1`, id).
		Scan(&no, &tp, &kegID, &batchID, &barID, &title, &desc, &status, &resolution, &createdAt, &resolvedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "事件不存在"})
		return
	}
	kegNo, batchNo, barName := "", "", ""
	if kegID != nil {
		db.QueryRow(`SELECT keg_no FROM kegs WHERE id=$1`, *kegID).Scan(&kegNo)
	}
	if batchID != nil {
		db.QueryRow(`SELECT batch_no FROM batches WHERE id=$1`, *batchID).Scan(&batchNo)
	}
	if barID != nil {
		db.QueryRow(`SELECT name FROM bars WHERE id=$1`, *barID).Scan(&barName)
	}
	actRows, _ := db.Query(`SELECT a.id,coalesce(u.name,'系统'),a.role,a.action,a.note,a.created_at
		FROM incident_actions a LEFT JOIN users u ON u.id=a.user_id
		WHERE a.incident_id=$1 ORDER BY a.id`, id)
	actions := []gin.H{}
	if actRows != nil {
		defer actRows.Close()
		for actRows.Next() {
			var aid int
			var name, r, action, note, ts string
			actRows.Scan(&aid, &name, &r, &action, &note, &ts)
			actions = append(actions, gin.H{"id": aid, "user_name": name, "role": r,
				"action": action, "note": note, "created_at": ts})
		}
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "incident_no": no, "type": tp, "keg_id": kegID, "keg_no": kegNo,
		"batch_id": batchID, "batch_no": batchNo, "bar_id": barID, "bar_name": barName,
		"title": title, "description": desc, "status": status, "resolution": resolution,
		"created_at": createdAt, "resolved_at": resolvedAt, "actions": actions})
}

// addIncidentAction 协同方在事件内处理（同一只桶/同一批酒）
func addIncidentAction(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Action == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "处理动作必填"})
		return
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM incidents WHERE id=$1`, id).Scan(&status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "事件不存在"})
		return
	}
	if status == "resolved" || status == "closed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "事件已办结"})
		return
	}
	u := uid(c)
	r := role(c)
	if _, err := db.Exec(`INSERT INTO incident_actions(incident_id,user_id,role,action,note) VALUES($1,$2,$3,$4,$5)`,
		id, u, r, req.Action, req.Note); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	db.Exec(`UPDATE incidents SET status='processing' WHERE id=$1 AND status='open'`, id)
	// 通知其他协同角色有新进展
	var title string
	db.QueryRow(`SELECT title FROM incidents WHERE id=$1`, id).Scan(&title)
	for _, rr := range []string{"warehouse", "qc", "finance", "admin"} {
		if rr != r {
			notifyRole(rr, "incident", "异常事件进展："+title, fmt.Sprintf("%s（%s）：%s", uname(c), r, req.Action), "incident", &id)
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// resolveIncident 办结事件：写入处理结论（押金扣款/再灌装资格/停售建议等）
func resolveIncident(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Resolution string `json:"resolution"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Resolution == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写处理结论"})
		return
	}
	res, err := db.Exec(`UPDATE incidents SET status='resolved', resolution=$1, resolved_at=now()
		WHERE id=$2 AND status IN ('open','processing')`, req.Resolution, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "事件已办结或不存在"})
		return
	}
	u := uid(c)
	r := role(c)
	db.Exec(`INSERT INTO incident_actions(incident_id,user_id,role,action,note) VALUES($1,$2,$3,'办结',$4)`,
		id, u, r, req.Resolution)
	var title string
	db.QueryRow(`SELECT title FROM incidents WHERE id=$1`, id).Scan(&title)
	for _, rr := range []string{"warehouse", "qc", "finance", "admin"} {
		notifyRole(rr, "incident", "异常事件已办结："+title, "处理结论："+req.Resolution, "incident", &id)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- 批次召回 ----------

func listRecalls(c *gin.Context) {
	rows, err := db.Query(`SELECT r.id,r.recall_no,r.batch_id,b.batch_no,b.beer_style,r.reason,r.level,r.status,r.created_at,
		(SELECT count(*) FROM recall_items i WHERE i.recall_id=r.id) AS total,
		(SELECT count(*) FROM recall_items i WHERE i.recall_id=r.id AND i.status!='pending') AS done
		FROM recalls r JOIN batches b ON b.id=r.batch_id ORDER BY r.id DESC`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, batchID, total, done int
		var no, batchNo, style, reason, level, status, createdAt string
		rows.Scan(&id, &no, &batchID, &batchNo, &style, &reason, &level, &status, &createdAt, &total, &done)
		list = append(list, gin.H{"id": id, "recall_no": no, "batch_id": batchID, "batch_no": batchNo,
			"beer_style": style, "reason": reason, "level": level, "status": status,
			"total_items": total, "done_items": done, "created_at": createdAt})
	}
	c.JSON(http.StatusOK, list)
}

// createRecall 发起召回：自动圈定范围（已售卖/已开桶/未开桶/在库/在途/经销商库存）
func createRecall(c *gin.Context) {
	var req struct {
		BatchID int    `json:"batch_id"`
		Reason  string `json:"reason"`
		Level   string `json:"level"` // 停售观察 / 全面召回
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BatchID == 0 || req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "批次和召回原因必填"})
		return
	}
	if req.Level == "" {
		req.Level = "全面召回"
	}
	var batchNo string
	var bStatus string
	if err := db.QueryRow(`SELECT batch_no,status FROM batches WHERE id=$1`, req.BatchID).Scan(&batchNo, &bStatus); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "批次不存在"})
		return
	}
	tx, _ := db.Begin()
	defer tx.Rollback()
	var recallID int
	var recallNo string
	tx.QueryRow(`SELECT 'RC-'||to_char(now(),'YYYYMMDD')||'-'||lpad((count(*)+1)::text,4,'0') FROM recalls`).Scan(&recallNo)
	u := uid(c)
	if err := tx.QueryRow(`INSERT INTO recalls(recall_no,batch_id,reason,level,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		recallNo, req.BatchID, req.Reason, req.Level, u).Scan(&recallID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 批次置为召回状态
	if _, err := tx.Exec(`UPDATE batches SET status='recalled', stop_reason=$1, stop_sale_at=now() WHERE id=$2`,
		"召回："+req.Reason, req.BatchID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 圈定召回范围
	kegRows, err := tx.Query(`SELECT id,status,location_type,coalesce(location_id,0) FROM kegs WHERE batch_id=$1`, req.BatchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type kk struct {
		id      int
		status  string
		locType string
		locID   int
	}
	var kegs []kk
	for kegRows.Next() {
		var k kk
		kegRows.Scan(&k.id, &k.status, &k.locType, &k.locID)
		kegs = append(kegs, k)
	}
	kegRows.Close()
	stateOf := func(k kk) (string, string, int) {
		switch k.status {
		case "delivered":
			return "unopened", "bar", k.locID
		case "opened":
			return "opened", "bar", k.locID
		case "empty", "pickup_scheduled", "returned", "clean_passed", "clean_failed":
			return "sold", "bar", k.locID // 已打空=已售卖消费
		case "filled":
			return "in_warehouse", "warehouse", 0
		case "in_transit":
			return "in_transit", "", 0
		case "at_distributor":
			return "at_distributor", "distributor", k.locID
		default:
			return "sold", "", 0
		}
	}
	counts := map[string]int{}
	barSet := map[int]bool{}
	distSet := map[int]bool{}
	for _, k := range kegs {
		state, ownerType, ownerID := stateOf(k)
		counts[state]++
		if ownerType == "bar" && state != "sold" {
			barSet[ownerID] = true
		}
		if ownerType == "distributor" {
			distSet[ownerID] = true
		}
		if _, err := tx.Exec(`INSERT INTO recall_items(recall_id,keg_id,state_at_recall,owner_type,owner_id)
			VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, recallID, k.id, state, ownerType, nullIfZero(ownerID)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 通知：已收货酒吧（未开桶/已开桶）、经销商、仓库、司机（待配送订单）、财务
	summary := fmt.Sprintf("已售卖 %d / 已开桶 %d / 未开桶 %d / 在库 %d / 在途 %d / 经销商 %d",
		counts["sold"], counts["opened"], counts["unopened"], counts["in_warehouse"], counts["in_transit"], counts["at_distributor"])
	for barID := range barSet {
		b := barID
		notify(nil, "", &b, "recall", fmt.Sprintf("批次 %s 召回通知（%s）", batchNo, req.Level),
			"召回原因："+req.Reason+"。请立即停止售卖该批次，未开桶酒桶请勿开桶，等待回收安排。", "recall", &recallID)
	}
	for distID := range distSet {
		var du *int
		db.QueryRow(`SELECT id FROM users WHERE role='distributor' AND org_id=$1 LIMIT 1`, distID).Scan(&du)
		if du != nil {
			notify(du, "", nil, "recall", fmt.Sprintf("批次 %s 召回通知（%s）", batchNo, req.Level),
				"召回原因："+req.Reason+"。请冻结该批次库存，停止向酒吧铺货。", "recall", &recallID)
		}
	}
	// 待配送订单中的该批次桶 → 通知仓库与司机
	orderRows, _ := db.Query(`SELECT DISTINCT o.id,o.order_no,o.driver_id FROM delivery_orders o
		JOIN delivery_order_items i ON i.order_id=o.id JOIN kegs k ON k.id=i.keg_id
		WHERE k.batch_id=$1 AND o.status IN ('pending','delivering')`, req.BatchID)
	if orderRows != nil {
		for orderRows.Next() {
			var oid int
			var ono string
			var drv *int
			orderRows.Scan(&oid, &ono, &drv)
			notifyRole("warehouse", "recall", fmt.Sprintf("订单 %s 含召回批次 %s，请暂停发货", ono, batchNo),
				"召回原因："+req.Reason, "order", &oid)
			if drv != nil {
				notify(drv, "", nil, "recall", fmt.Sprintf("订单 %s 含召回批次 %s，请暂停配送", ono, batchNo),
					"召回原因："+req.Reason, "order", &oid)
			}
		}
		orderRows.Close()
	}
	notifyRole("finance", "recall", fmt.Sprintf("批次 %s 已召回", batchNo), summary, "recall", &recallID)
	notifyRole("warehouse", "recall", fmt.Sprintf("批次 %s 已召回", batchNo), summary, "recall", &recallID)
	c.JSON(http.StatusOK, gin.H{"id": recallID, "recall_no": recallNo, "scope": counts})
}

func nullIfZero(v int) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func recallDetail(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var no, reason, level, status, createdAt string
	var batchID int
	err := db.QueryRow(`SELECT recall_no,batch_id,reason,level,status,created_at FROM recalls WHERE id=$1`, id).
		Scan(&no, &batchID, &reason, &level, &status, &createdAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "召回单不存在"})
		return
	}
	var batchNo, style string
	db.QueryRow(`SELECT batch_no,beer_style FROM batches WHERE id=$1`, batchID).Scan(&batchNo, &style)
	itemRows, _ := db.Query(`SELECT i.id,i.keg_id,k.keg_no,i.state_at_recall,i.owner_type,coalesce(i.owner_id,0),i.status,i.note
		FROM recall_items i JOIN kegs k ON k.id=i.keg_id WHERE i.recall_id=$1 ORDER BY i.state_at_recall, i.id`, id)
	items := []gin.H{}
	stateCount := map[string]int{}
	if itemRows != nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var iid, kegID, ownerID int
			var kegNo, state, ownerType, istatus, note string
			itemRows.Scan(&iid, &kegID, &kegNo, &state, &ownerType, &ownerID, &istatus, &note)
			stateCount[state]++
			ownerName := ""
			if ownerType == "bar" {
				db.QueryRow(`SELECT name FROM bars WHERE id=$1`, ownerID).Scan(&ownerName)
			} else if ownerType == "distributor" {
				db.QueryRow(`SELECT name FROM distributors WHERE id=$1`, ownerID).Scan(&ownerName)
			} else if ownerType == "warehouse" {
				ownerName = "酒厂仓库"
			}
			items = append(items, gin.H{"id": iid, "keg_id": kegID, "keg_no": kegNo, "state": state,
				"owner_type": ownerType, "owner_id": ownerID, "owner_name": ownerName,
				"status": istatus, "note": note})
		}
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "recall_no": no, "batch_id": batchID, "batch_no": batchNo,
		"beer_style": style, "reason": reason, "level": level, "status": status,
		"created_at": createdAt, "items": items, "state_count": stateCount})
}

// updateRecallItem 更新召回明细处理状态
func updateRecallItem(c *gin.Context) {
	itemID, _ := strconv.Atoi(c.Param("itemId"))
	var req struct {
		Status string `json:"status"` // returned / destroyed / exempted
		Note   string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil ||
		(req.Status != "returned" && req.Status != "destroyed" && req.Status != "exempted") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "状态无效"})
		return
	}
	var recallID, kegID int
	err := db.QueryRow(`SELECT recall_id,keg_id FROM recall_items WHERE id=$1`, itemID).Scan(&recallID, &kegID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "明细不存在"})
		return
	}
	u := uid(c)
	tx, _ := db.Begin()
	defer tx.Rollback()
	tx.Exec(`UPDATE recall_items SET status=$1, note=$2 WHERE id=$3`, req.Status, req.Note, itemID)
	// 已退回的桶回收入库待清洗
	if req.Status == "returned" {
		tx.Exec(`UPDATE kegs SET status='returned', location_type='cleaning_plant', location_id=NULL WHERE id=$1`, kegID)
		addKegEvent(tx, kegID, "召回退回", req.Note, &u)
	}
	if req.Status == "destroyed" {
		tx.Exec(`UPDATE kegs SET status='scrapped', location_type='none', location_id=NULL WHERE id=$1`, kegID)
		addKegEvent(tx, kegID, "召回销毁", req.Note, &u)
	}
	tx.Commit()
	// 全部处理完 → 召回完成
	var pending int
	db.QueryRow(`SELECT count(*) FROM recall_items WHERE recall_id=$1 AND status='pending'`, recallID).Scan(&pending)
	if pending == 0 {
		db.Exec(`UPDATE recalls SET status='completed' WHERE id=$1`, recallID)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "remaining": pending})
}

// ---------- 通知 ----------

func listNotifications(c *gin.Context) {
	u := uid(c)
	r := role(c)
	var barID *int
	if r == "bar_manager" {
		if org, ok := c.Get("org_id"); ok {
			b := org.(int)
			barID = &b
		}
	}
	rows, err := db.Query(`SELECT n.id,n.type,n.title,n.content,n.ref_type,coalesce(n.ref_id,0),n.created_at,
		EXISTS(SELECT 1 FROM notification_reads rd WHERE rd.notification_id=n.id AND rd.user_id=$1) AS read
		FROM notifications n
		WHERE n.user_id=$1 OR (n.user_id IS NULL AND n.role=$2) OR (n.user_id IS NULL AND n.bar_id IS NOT NULL AND n.bar_id=$3)
		ORDER BY n.id DESC LIMIT 100`, u, r, barID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []gin.H{}
	for rows.Next() {
		var id, refID int
		var tp, title, content, refType, createdAt string
		var read bool
		rows.Scan(&id, &tp, &title, &content, &refType, &refID, &createdAt, &read)
		list = append(list, gin.H{"id": id, "type": tp, "title": title, "content": content,
			"ref_type": refType, "ref_id": refID, "created_at": createdAt, "read": read})
	}
	c.JSON(http.StatusOK, list)
}

func markNotificationRead(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	db.Exec(`INSERT INTO notification_reads(notification_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, uid(c))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func unreadCount(c *gin.Context) {
	u := uid(c)
	r := role(c)
	var barID *int
	if r == "bar_manager" {
		if org, ok := c.Get("org_id"); ok {
			b := org.(int)
			barID = &b
		}
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM notifications n
		WHERE (n.user_id=$1 OR (n.user_id IS NULL AND n.role=$2) OR (n.user_id IS NULL AND n.bar_id=$3))
		  AND NOT EXISTS(SELECT 1 FROM notification_reads rd WHERE rd.notification_id=n.id AND rd.user_id=$1)`,
		u, r, barID).Scan(&n)
	c.JSON(http.StatusOK, gin.H{"count": n})
}

// ---------- 仪表盘 ----------

func dashboard(c *gin.Context) {
	checkLateReturns() // 每次拉取仪表盘时扫描超期未还
	out := gin.H{}
	// 酒桶状态分布
	stateRows, _ := db.Query(`SELECT status,count(*) FROM kegs GROUP BY status`)
	states := map[string]int{}
	if stateRows != nil {
		for stateRows.Next() {
			var s string
			var n int
			stateRows.Scan(&s, &n)
			states[s] = n
		}
		stateRows.Close()
	}
	out["keg_states"] = states
	out["keg_state_names"] = kegStatusNames
	var batchTotal, batchSuspended, batchRecalled int
	db.QueryRow(`SELECT count(*),
		count(*) FILTER (WHERE status='suspended'),
		count(*) FILTER (WHERE status='recalled') FROM batches`).Scan(&batchTotal, &batchSuspended, &batchRecalled)
	out["batch_total"] = batchTotal
	out["batch_suspended"] = batchSuspended
	out["batch_recalled"] = batchRecalled
	var openIncidents int
	db.QueryRow(`SELECT count(*) FROM incidents WHERE status IN ('open','processing')`).Scan(&openIncidents)
	out["open_incidents"] = openIncidents
	var pendingOrders, deliveringOrders int
	db.QueryRow(`SELECT count(*) FILTER (WHERE status='pending'), count(*) FILTER (WHERE status='delivering')
		FROM delivery_orders`).Scan(&pendingOrders, &deliveringOrders)
	out["pending_orders"] = pendingOrders
	out["delivering_orders"] = deliveringOrders
	var depositHeld float64
	db.QueryRow(`SELECT coalesce(sum(balance),0) FROM deposit_accounts`).Scan(&depositHeld)
	out["deposit_held"] = depositHeld
	var overdueTx int
	db.QueryRow(`SELECT count(*) FROM deposit_transactions WHERE status='pending' AND due_date < CURRENT_DATE`).Scan(&overdueTx)
	out["overdue_deposit_tx"] = overdueTx
	var activeRecalls int
	db.QueryRow(`SELECT count(*) FROM recalls WHERE status='active'`).Scan(&activeRecalls)
	out["active_recalls"] = activeRecalls
	var cleaningQueue int
	db.QueryRow(`SELECT count(*) FROM kegs WHERE status='returned'`).Scan(&cleaningQueue)
	out["cleaning_queue"] = cleaningQueue
	// 酒吧视角
	if role(c) == "bar_manager" {
		if org, ok := c.Get("org_id"); ok {
			barID := org.(int)
			myStates := map[string]int{}
			rows, _ := db.Query(`SELECT status,count(*) FROM kegs WHERE location_type='bar' AND location_id=$1 GROUP BY status`, barID)
			if rows != nil {
				for rows.Next() {
					var s string
					var n int
					rows.Scan(&s, &n)
					myStates[s] = n
				}
				rows.Close()
			}
			out["my_keg_states"] = myStates
			var myDeposit float64
			db.QueryRow(`SELECT coalesce(balance,0) FROM deposit_accounts WHERE owner_type='bar' AND owner_id=$1`, barID).Scan(&myDeposit)
			out["my_deposit"] = myDeposit
		}
	}
	c.JSON(http.StatusOK, out)
}
