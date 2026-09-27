package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

var db *sql.DB

//go:embed schema.sql
var schemaSQL string

func mustGetenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func initDB() {
	dsn := mustGetenv("DATABASE_URL",
		"host=127.0.0.1 port=5433 user=postgres dbname=kegtrace sslmode=disable")
	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(20)
	if err = db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	if _, err = db.Exec(schemaSQL); err != nil {
		log.Fatalf("apply schema: %v", err)
	}
	fmt.Println("database ready")
}

// ---------- 通用辅助 ----------

// addKegEvent 记录酒桶生命周期事件（追溯时间线）
func addKegEvent(tx *sql.Tx, kegID int, event, detail string, actorID *int) error {
	var err error
	if tx != nil {
		_, err = tx.Exec(`INSERT INTO keg_events(keg_id,event,detail,actor_id) VALUES($1,$2,$3,$4)`,
			kegID, event, detail, actorID)
	} else {
		_, err = db.Exec(`INSERT INTO keg_events(keg_id,event,detail,actor_id) VALUES($1,$2,$3,$4)`,
			kegID, event, detail, actorID)
	}
	return err
}

// notify 发送通知：可指定 userID / role / barID 之一
func notify(userID *int, role string, barID *int, ntype, title, content, refType string, refID *int) {
	_, err := db.Exec(`INSERT INTO notifications(user_id,role,bar_id,type,title,content,ref_type,ref_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		userID, role, barID, ntype, title, content, refType, refID)
	if err != nil {
		log.Printf("notify error: %v", err)
	}
}

// notifyRole 给某角色全体发通知
func notifyRole(role, ntype, title, content, refType string, refID *int) {
	notify(nil, role, nil, ntype, title, content, refType, refID)
}

// ensureDepositAccount 获取或创建押金账户
func ensureDepositAccount(tx *sql.Tx, ownerType string, ownerID int) (int, error) {
	var id int
	q := `INSERT INTO deposit_accounts(owner_type,owner_id) VALUES($1,$2)
		ON CONFLICT(owner_type,owner_id) DO UPDATE SET owner_type=EXCLUDED.owner_type
		RETURNING id`
	var err error
	if tx != nil {
		err = tx.QueryRow(q, ownerType, ownerID).Scan(&id)
	} else {
		err = db.QueryRow(q, ownerType, ownerID).Scan(&id)
	}
	return id, err
}

// createIncident 创建异常事件并通知相关角色
func createIncident(itype string, kegID, batchID, barID *int, title, description string) (int, error) {
	var id int
	var no string
	err := db.QueryRow(`SELECT 'INC-'||to_char(now(),'YYYYMMDD')||'-'||lpad((count(*)+1)::text,4,'0') FROM incidents`).Scan(&no)
	if err != nil {
		return 0, err
	}
	err = db.QueryRow(`INSERT INTO incidents(incident_no,type,keg_id,batch_id,bar_id,title,description)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, no, itype, kegID, batchID, barID, title, description).Scan(&id)
	if err != nil {
		return 0, err
	}
	// 异常事件通知所有协同方角色
	for _, r := range []string{"warehouse", "qc", "finance", "admin"} {
		notifyRole(r, "incident", "新异常事件："+title, description, "incident", &id)
	}
	return id, nil
}

// checkBatchComplaints 检查批次是否达到多家投诉阈值，触发批次投诉异常
func checkBatchComplaints(batchID int) {
	var batchNo string
	if err := db.QueryRow(`SELECT batch_no FROM batches WHERE id=$1`, batchID).Scan(&batchNo); err != nil {
		return
	}
	// 明显及以上口味异常或客诉，且来自 >=2 家不同酒吧
	var barCount, fbCount int
	err := db.QueryRow(`SELECT count(DISTINCT bar_id), count(*) FROM feedbacks
		WHERE batch_id=$1 AND (taste_anomaly IN ('明显','严重') OR complaint=TRUE)`, batchID).Scan(&barCount, &fbCount)
	if err != nil || barCount < 2 {
		return
	}
	// 已有未关闭的同批次投诉事件则不重复创建
	var existing int
	db.QueryRow(`SELECT count(*) FROM incidents WHERE type='batch_complaint' AND batch_id=$1 AND status IN ('open','processing')`,
		batchID).Scan(&existing)
	if existing > 0 {
		return
	}
	title := fmt.Sprintf("批次 %s 遭 %d 家酒吧投诉（共 %d 条严重反馈）", batchNo, barCount, fbCount)
	desc := "多家酒吧反馈该批次存在明显口味异常或客诉，建议品控评估停售。"
	id, err := createIncident("batch_complaint", nil, &batchID, nil, title, desc)
	if err == nil {
		notifyRole("qc", "incident", title, desc+" 请在异常事件中给出停售建议。", "incident", &id)
	}
}

// checkLateReturns 扫描超期未归还酒桶，生成延迟归还异常（幂等）
func checkLateReturns() {
	rows, err := db.Query(`SELECT k.id, k.keg_no, k.location_id, b.name
		FROM kegs k JOIN bars b ON b.id=k.location_id
		WHERE k.location_type='bar' AND k.status IN ('delivered','opened','empty','pickup_scheduled')
		  AND k.delivered_at < now() - interval '30 days'
		  AND NOT EXISTS (SELECT 1 FROM incidents i WHERE i.type='late_return' AND i.keg_id=k.id AND i.status IN ('open','processing'))`)
	if err != nil {
		return
	}
	defer rows.Close()
	type lr struct {
		id      int
		kegNo   string
		barID   int
		barName string
	}
	var list []lr
	for rows.Next() {
		var x lr
		if rows.Scan(&x.id, &x.kegNo, &x.barID, &x.barName) == nil {
			list = append(list, x)
		}
	}
	for _, x := range list {
		title := fmt.Sprintf("酒桶 %s 超期未归还（%s）", x.kegNo, x.barName)
		kegID, barID := x.id, x.barID
		createIncident("late_return", &kegID, nil, &barID, title,
			"酒桶在酒吧超过 30 天未完成回收，请财务评估押金处理，仓库跟进回收。")
	}
}
