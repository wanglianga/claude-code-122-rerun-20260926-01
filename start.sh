#!/usr/bin/env bash
# 一键启动：PostgreSQL(工作区实例) + Go 后端(8080) + 前端静态服务
set -e
cd "$(dirname "$0")"

PGDATA="$PWD/.pgdata"
PGBIN="${PGBIN:-/usr/lib/postgresql/15/bin}"

# 1. 数据库：优先使用工作区自带的 PostgreSQL 实例（端口 5433）
if [ -d "$PGDATA" ]; then
  "$PGBIN/pg_ctl" -D "$PGDATA" -l /tmp/kegtrace-pg.log -w start || true
else
  echo "未找到 $PGDATA，请确保 PostgreSQL 已启动并创建 kegtrace 数据库"
fi

# 2. 后端（首次自动建表 + 写入演示数据）
cd server
go build -o ../kegtrace-server .
cd ..
export DATABASE_URL="${DATABASE_URL:-host=127.0.0.1 port=5433 user=postgres dbname=kegtrace sslmode=disable}"

# 3. 前端（已构建则由后端直接托管）
if [ ! -d web/dist ]; then
  cd web && npm install && npm run build && cd ..
fi
export WEB_DIST="${WEB_DIST:-$PWD/web/dist}"

echo "启动平台：http://localhost:8080"
exec ./kegtrace-server
