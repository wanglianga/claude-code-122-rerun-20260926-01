package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func main() {
	initDB()
	seedIfEmpty()

	r := gin.Default()

	api := r.Group("/api")
	api.POST("/login", loginHandler)

	auth := api.Group("")
	auth.Use(authMiddleware())
	{
		auth.GET("/me", meHandler)
		auth.POST("/logout", logoutHandler)
		auth.GET("/dashboard", dashboard)

		// 基础数据
		auth.GET("/bars", listBars)
		auth.POST("/bars", requireRole("admin", "warehouse"), createBar)
		auth.GET("/distributors", listDistributors)
		auth.POST("/distributors", requireRole("admin", "warehouse"), createDistributor)
		auth.GET("/users", requireRole("admin"), listUsers)
		auth.GET("/drivers", listDrivers)

		// 批次
		auth.GET("/batches", listBatches)
		auth.POST("/batches", requireRole("warehouse", "qc"), createBatch)
		auth.GET("/batches/:id", batchDetail)
		auth.POST("/batches/:id/suspend", requireRole("qc", "admin"), suspendBatch)
		auth.POST("/batches/:id/resume", requireRole("qc", "admin"), resumeBatch)

		// 酒桶
		auth.GET("/kegs", listKegs)
		auth.POST("/kegs", requireRole("warehouse"), createKeg)
		auth.GET("/kegs/:id", kegDetail)
		auth.POST("/kegs/:id/refill", requireRole("warehouse"), refillKeg)
		auth.POST("/kegs/:id/open", requireRole("bar_manager"), openKeg)
		auth.POST("/kegs/:id/empty", requireRole("bar_manager"), emptyKeg)
		auth.POST("/kegs/:id/report-lost", reportLost)

		// 配送订单
		auth.GET("/orders", listOrders)
		auth.POST("/orders", requireRole("warehouse", "distributor"), createOrder)
		auth.GET("/orders/:id", orderDetail)
		auth.POST("/orders/:id/dispatch", requireRole("driver", "warehouse"), dispatchOrder)
		auth.POST("/orders/:id/deliver", requireRole("driver", "warehouse"), deliverOrder)
		auth.POST("/orders/:id/cancel", requireRole("warehouse", "distributor"), cancelOrder)

		// 反馈
		auth.POST("/feedbacks", requireRole("bar_manager"), createFeedback)

		// 回收预约与扫码
		auth.GET("/appointments", listAppointments)
		auth.POST("/appointments", requireRole("bar_manager"), createAppointment)
		auth.GET("/appointments/:id", appointmentDetail)
		auth.POST("/appointments/:id/claim", requireRole("driver"), claimAppointment)
		auth.POST("/appointments/:id/scan", requireRole("driver"), scanKeg)
		auth.POST("/appointments/:id/complete", requireRole("driver"), completeAppointment)

		// 清洗厂
		auth.GET("/cleaning/queue", requireRole("cleaner", "warehouse"), listCleaningQueue)
		auth.POST("/cleaning", requireRole("cleaner"), recordCleaning)

		// 押金财务
		auth.GET("/deposits/accounts", requireRole("finance", "bar_manager", "distributor"), listDepositAccounts)
		auth.GET("/deposits/accounts/:id/transactions", requireRole("finance", "bar_manager", "distributor"), listDepositTransactions)
		auth.POST("/deposits/settle", requireRole("finance"), settleDeposit)

		// 异常事件
		auth.GET("/incidents", listIncidents)
		auth.GET("/incidents/:id", incidentDetail)
		auth.POST("/incidents/:id/actions", addIncidentAction)
		auth.POST("/incidents/:id/resolve", requireRole("admin", "qc", "finance", "warehouse"), resolveIncident)

		// 召回
		auth.GET("/recalls", listRecalls)
		auth.POST("/recalls", requireRole("qc", "admin"), createRecall)
		auth.GET("/recalls/:id", recallDetail)
		auth.POST("/recalls/:id/items/:itemId", requireRole("warehouse", "qc", "admin"), updateRecallItem)

		// 通知
		auth.GET("/notifications", listNotifications)
		auth.GET("/notifications/unread-count", unreadCount)
		auth.POST("/notifications/:id/read", markNotificationRead)
	}

	// 静态文件：生产模式服务前端构建产物
	distDir := os.Getenv("WEB_DIST")
	if distDir == "" {
		distDir = "../web/dist"
	}
	if _, err := os.Stat(distDir); err == nil {
		r.NoRoute(func(c *gin.Context) {
			p := c.Request.URL.Path
			if strings.HasPrefix(p, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			fp := filepath.Join(distDir, p)
			if st, err := os.Stat(fp); err == nil && !st.IsDir() {
				c.File(fp)
				return
			}
			c.File(filepath.Join(distDir, "index.html"))
		})
		log.Printf("serving frontend from %s", distDir)
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
