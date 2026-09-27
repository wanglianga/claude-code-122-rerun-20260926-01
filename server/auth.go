package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type Session struct {
	UserID int
	Role   string
	Name   string
	OrgID  *int
}

var (
	sessionsMu sync.RWMutex
	sessions   = map[string]Session{}
)

func newToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type CurrentUser struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	OrgID  *int   `json:"org_id"`
	BarID  *int   `json:"bar_id,omitempty"`
	BarName string `json:"bar_name,omitempty"`
}

// authMiddleware 解析 Bearer token
func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if len(h) < 8 || h[:7] != "Bearer " {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		token := h[7:]
		sessionsMu.RLock()
		s, ok := sessions[token]
		sessionsMu.RUnlock()
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
			return
		}
		c.Set("uid", s.UserID)
		c.Set("role", s.Role)
		c.Set("uname", s.Name)
		if s.OrgID != nil {
			c.Set("org_id", *s.OrgID)
		}
		c.Next()
	}
}

func uid(c *gin.Context) int      { return c.GetInt("uid") }
func role(c *gin.Context) string  { return c.GetString("role") }
func uname(c *gin.Context) string { return c.GetString("uname") }

func requireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		r := role(c)
		for _, x := range roles {
			if x == r || r == "admin" {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权限"})
	}
}

func loginHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	var id int
	var hash, name, r string
	var orgID *int
	err := db.QueryRow(`SELECT id,password_hash,name,role,org_id FROM users WHERE username=$1`,
		req.Username).Scan(&id, &hash, &name, &r, &orgID)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	token := newToken()
	sessionsMu.Lock()
	sessions[token] = Session{UserID: id, Role: r, Name: name, OrgID: orgID}
	sessionsMu.Unlock()
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  gin.H{"id": id, "name": name, "role": r, "org_id": orgID},
	})
}

func meHandler(c *gin.Context) {
	var orgID *int
	if v, ok := c.Get("org_id"); ok {
		x := v.(int)
		orgID = &x
	}
	u := gin.H{"id": uid(c), "name": uname(c), "role": role(c), "org_id": orgID}
	if role(c) == "bar_manager" && orgID != nil {
		var barName string
		if db.QueryRow(`SELECT name FROM bars WHERE id=$1`, *orgID).Scan(&barName) == nil {
			u["bar_name"] = barName
		}
	}
	if role(c) == "distributor" && orgID != nil {
		var dn string
		if db.QueryRow(`SELECT name FROM distributors WHERE id=$1`, *orgID).Scan(&dn) == nil {
			u["distributor_name"] = dn
		}
	}
	c.JSON(http.StatusOK, u)
}

func logoutHandler(c *gin.Context) {
	h := c.GetHeader("Authorization")
	if len(h) >= 8 {
		sessionsMu.Lock()
		delete(sessions, h[7:])
		sessionsMu.Unlock()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
