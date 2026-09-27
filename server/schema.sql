-- 精酿酒吧空桶回收押金与批次追溯平台 - 数据库模式
CREATE TABLE IF NOT EXISTS users (
    id            SERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('admin','warehouse','bar_manager','driver','cleaner','qc','finance','distributor')),
    org_id        INT,                -- bar_id 或 distributor_id（视角色而定）
    phone         TEXT DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS distributors (
    id         SERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    city       TEXT NOT NULL,
    contact    TEXT DEFAULT '',
    phone      TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bars (
    id                       SERIAL PRIMARY KEY,
    name                     TEXT NOT NULL,
    city                     TEXT NOT NULL,
    address                  TEXT DEFAULT '',
    manager_name             TEXT DEFAULT '',
    phone                    TEXT DEFAULT '',
    distributor_id           INT REFERENCES distributors(id),  -- 所属经销商（空=酒厂直供）
    deposit_credit_term_days INT NOT NULL DEFAULT 30,          -- 押金账期（天）
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS batches (
    id           SERIAL PRIMARY KEY,
    batch_no     TEXT NOT NULL UNIQUE,
    beer_style   TEXT NOT NULL CHECK (beer_style IN ('IPA','世涛','小麦','酸啤')),
    brew_date    DATE NOT NULL,
    fill_date    DATE NOT NULL,
    best_before  DATE NOT NULL,          -- 最佳饮用期
    abv          NUMERIC(4,2) DEFAULT 0,
    status       TEXT NOT NULL DEFAULT 'normal' CHECK (status IN ('normal','suspended','recalled')),
    stop_reason  TEXT DEFAULT '',
    stop_sale_at TIMESTAMPTZ,            -- 停售时间
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 酒桶状态机：
-- filled 在库已灌装 / in_transit 配送在途 / at_distributor 经销商库存
-- delivered 酒吧未开桶 / opened 已开桶 / empty 已打空待回收 / pickup_scheduled 已预约回收
-- returned 已回收待清洗 / clean_passed 清洗合格可再灌装 / clean_failed 清洗不合格
-- lost 丢失 / scrapped 报废
CREATE TABLE IF NOT EXISTS kegs (
    id             SERIAL PRIMARY KEY,
    keg_no         TEXT NOT NULL UNIQUE,          -- 桶身扫码编号
    capacity_liter NUMERIC(5,1) NOT NULL,         -- 容量（升）
    batch_id       INT REFERENCES batches(id),
    deposit_amount NUMERIC(10,2) NOT NULL,        -- 单桶押金
    status         TEXT NOT NULL DEFAULT 'filled',
    location_type  TEXT NOT NULL DEFAULT 'warehouse',  -- warehouse/distributor/bar/in_transit/cleaning_plant/none
    location_id    INT,                            -- 对应 distributor_id 或 bar_id
    current_order_id INT,
    opened_at      TIMESTAMPTZ,
    emptied_at     TIMESTAMPTZ,
    delivered_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 配送订单：酒厂→经销商 / 酒厂→酒吧 / 经销商→酒吧
CREATE TABLE IF NOT EXISTS delivery_orders (
    id                 SERIAL PRIMARY KEY,
    order_no           TEXT NOT NULL UNIQUE,
    from_type          TEXT NOT NULL CHECK (from_type IN ('brewery','distributor')),
    from_distributor_id INT REFERENCES distributors(id),
    to_type            TEXT NOT NULL CHECK (to_type IN ('distributor','bar')),
    to_distributor_id  INT REFERENCES distributors(id),
    to_bar_id          INT REFERENCES bars(id),
    driver_id          INT REFERENCES users(id),
    status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','delivering','delivered','cancelled')),
    is_holiday_temp    BOOLEAN NOT NULL DEFAULT FALSE,  -- 节日临时铺货
    note               TEXT DEFAULT '',
    receiver_name      TEXT DEFAULT '',                 -- 酒吧收货人
    created_by         INT REFERENCES users(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at       TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS delivery_order_items (
    id       SERIAL PRIMARY KEY,
    order_id INT NOT NULL REFERENCES delivery_orders(id) ON DELETE CASCADE,
    keg_id   INT NOT NULL REFERENCES kegs(id),
    UNIQUE(order_id, keg_id)
);

-- 酒吧开桶反馈
CREATE TABLE IF NOT EXISTS feedbacks (
    id             SERIAL PRIMARY KEY,
    keg_id         INT NOT NULL REFERENCES kegs(id),
    bar_id         INT NOT NULL REFERENCES bars(id),
    batch_id       INT NOT NULL REFERENCES batches(id),
    pour_speed     TEXT NOT NULL DEFAULT '正常',     -- 出酒速度：正常/偏慢/偏快/不出酒
    pressure_issue TEXT NOT NULL DEFAULT '正常',     -- 气压：正常/气压不足/气压过高
    taste_anomaly  TEXT NOT NULL DEFAULT '无',       -- 口味异常：无/轻微/明显/严重
    taste_note     TEXT DEFAULT '',
    complaint      BOOLEAN NOT NULL DEFAULT FALSE,   -- 是否客诉
    complaint_note TEXT DEFAULT '',
    created_by     INT REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 回收预约
CREATE TABLE IF NOT EXISTS pickup_appointments (
    id             SERIAL PRIMARY KEY,
    appt_no        TEXT NOT NULL UNIQUE,
    bar_id         INT NOT NULL REFERENCES bars(id),
    status         TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled','scanning','completed','cancelled')),
    scheduled_date DATE NOT NULL,
    driver_id      INT REFERENCES users(id),
    note           TEXT DEFAULT '',
    created_by     INT REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at   TIMESTAMPTZ
);

-- 司机回收扫码记录
CREATE TABLE IF NOT EXISTS pickup_scans (
    id             SERIAL PRIMARY KEY,
    appointment_id INT NOT NULL REFERENCES pickup_appointments(id) ON DELETE CASCADE,
    keg_id         INT NOT NULL REFERENCES kegs(id),
    scanned_by     INT REFERENCES users(id),
    appearance     TEXT NOT NULL DEFAULT '完好',   -- 完好/轻微磕碰/桶体变形
    residual       TEXT NOT NULL DEFAULT '无残液', -- 无残液/少量/较多
    cap_status     TEXT NOT NULL DEFAULT '封盖完好', -- 封盖完好/封盖缺失/封盖损坏
    scanned_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(appointment_id, keg_id)
);

-- 清洗厂检测记录
CREATE TABLE IF NOT EXISTS cleaning_records (
    id         SERIAL PRIMARY KEY,
    keg_id     INT NOT NULL REFERENCES kegs(id),
    operator_id INT REFERENCES users(id),
    result     TEXT NOT NULL CHECK (result IN ('passed','failed')),
    check_note TEXT DEFAULT '',   -- 内窥/残碱/异味/压力等检测说明
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 押金账户（酒吧或经销商）
CREATE TABLE IF NOT EXISTS deposit_accounts (
    id          SERIAL PRIMARY KEY,
    owner_type  TEXT NOT NULL CHECK (owner_type IN ('bar','distributor')),
    owner_id    INT NOT NULL,
    balance     NUMERIC(12,2) NOT NULL DEFAULT 0,  -- 在押押金总额
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(owner_type, owner_id)
);

-- 押金流水：hold 收取 / release 释放 / deduct 扣款 / adjust 调整
CREATE TABLE IF NOT EXISTS deposit_transactions (
    id          SERIAL PRIMARY KEY,
    account_id  INT NOT NULL REFERENCES deposit_accounts(id),
    keg_id      INT REFERENCES kegs(id),
    type        TEXT NOT NULL CHECK (type IN ('hold','release','deduct','adjust')),
    amount      NUMERIC(12,2) NOT NULL,
    reason      TEXT DEFAULT '',
    ref_type    TEXT DEFAULT '',     -- order/incident/manual
    ref_id      INT,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','settled')),
    due_date    DATE,                -- 账期到期日
    created_by  INT REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 异常事件（多方协同处理）
CREATE TABLE IF NOT EXISTS incidents (
    id           SERIAL PRIMARY KEY,
    incident_no  TEXT NOT NULL UNIQUE,
    type         TEXT NOT NULL CHECK (type IN ('late_return','lost','deformed','batch_complaint','missed_scan','cleaning_failed')),
    keg_id       INT REFERENCES kegs(id),
    batch_id     INT REFERENCES batches(id),
    bar_id       INT REFERENCES bars(id),
    title        TEXT NOT NULL,
    description  TEXT DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','processing','resolved','closed')),
    resolution   TEXT DEFAULT '',     -- 处理结论（押金扣款/再灌装资格/停售建议等）
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at  TIMESTAMPTZ
);

-- 异常事件协同处理动作（仓库/酒吧/司机/清洗厂/品控/财务同事件协作）
CREATE TABLE IF NOT EXISTS incident_actions (
    id          SERIAL PRIMARY KEY,
    incident_id INT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    user_id     INT REFERENCES users(id),
    role        TEXT NOT NULL,
    action      TEXT NOT NULL,
    note        TEXT DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 批次召回
CREATE TABLE IF NOT EXISTS recalls (
    id         SERIAL PRIMARY KEY,
    recall_no  TEXT NOT NULL UNIQUE,
    batch_id   INT NOT NULL REFERENCES batches(id),
    reason     TEXT NOT NULL,
    level      TEXT NOT NULL DEFAULT '停售观察' CHECK (level IN ('停售观察','全面召回')),
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','completed')),
    created_by INT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 召回范围明细：区分 已售卖/已开桶/未开桶/在库/在途/经销商库存
CREATE TABLE IF NOT EXISTS recall_items (
    id             SERIAL PRIMARY KEY,
    recall_id      INT NOT NULL REFERENCES recalls(id) ON DELETE CASCADE,
    keg_id         INT NOT NULL REFERENCES kegs(id),
    state_at_recall TEXT NOT NULL,   -- sold/opened/unopened/in_warehouse/in_transit/at_distributor
    owner_type     TEXT DEFAULT '',  -- bar/distributor/warehouse
    owner_id       INT,
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','returned','destroyed','exempted')),
    note           TEXT DEFAULT '',
    UNIQUE(recall_id, keg_id)
);

-- 通知
CREATE TABLE IF NOT EXISTS notifications (
    id         SERIAL PRIMARY KEY,
    user_id    INT REFERENCES users(id),   -- 指定用户；为空则按 role 广播
    role       TEXT DEFAULT '',            -- 指定角色广播
    bar_id     INT REFERENCES bars(id),    -- 指定酒吧（该酒吧所有店长账号）
    type       TEXT NOT NULL,              -- recall/stop_sale/incident/deposit/pickup/order
    title      TEXT NOT NULL,
    content    TEXT DEFAULT '',
    ref_type   TEXT DEFAULT '',
    ref_id     INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notification_reads (
    id              SERIAL PRIMARY KEY,
    notification_id INT NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    user_id         INT NOT NULL REFERENCES users(id),
    read_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(notification_id, user_id)
);

-- 酒桶全生命周期事件（追溯时间线）
CREATE TABLE IF NOT EXISTS keg_events (
    id         SERIAL PRIMARY KEY,
    keg_id     INT NOT NULL REFERENCES kegs(id),
    event      TEXT NOT NULL,
    detail     TEXT DEFAULT '',
    actor_id   INT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_kegs_batch ON kegs(batch_id);
CREATE INDEX IF NOT EXISTS idx_kegs_status ON kegs(status);
CREATE INDEX IF NOT EXISTS idx_kegs_location ON kegs(location_type, location_id);
CREATE INDEX IF NOT EXISTS idx_feedbacks_batch ON feedbacks(batch_id);
CREATE INDEX IF NOT EXISTS idx_keg_events_keg ON keg_events(keg_id);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_role ON notifications(role);
CREATE INDEX IF NOT EXISTS idx_deposit_tx_account ON deposit_transactions(account_id);
