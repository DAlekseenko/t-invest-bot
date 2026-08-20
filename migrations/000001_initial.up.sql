CREATE TABLE broker_accounts (
    id UUID PRIMARY KEY,
    alias TEXT NOT NULL UNIQUE,
    broker_account_id_ciphertext TEXT NOT NULL,
    environment TEXT NOT NULL CHECK (environment IN ('sandbox', 'prod')),
    enabled BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE instruments (
    instrument_uid TEXT PRIMARY KEY,
    figi TEXT NOT NULL,
    ticker TEXT NOT NULL,
    class_code TEXT NOT NULL,
    lot BIGINT NOT NULL CHECK (lot > 0),
    currency TEXT NOT NULL,
    min_price_increment_nano BIGINT NOT NULL CHECK (min_price_increment_nano > 0),
    api_trade_available BOOLEAN NOT NULL,
    metadata_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE config_snapshots (
    id UUID PRIMARY KEY,
    schema_version BIGINT NOT NULL CHECK (schema_version > 0),
    revision BIGINT NOT NULL UNIQUE CHECK (revision > 0),
    content_hash CHAR(64) NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    mode TEXT NOT NULL CHECK (mode IN ('disabled', 'dry_run', 'sandbox', 'live')),
    approved BOOLEAN NOT NULL,
    payload JSONB NOT NULL,
    published_at TIMESTAMPTZ NOT NULL,
    loaded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE strategies (
    id UUID PRIMARY KEY,
    external_strategy_id TEXT NOT NULL,
    account_id UUID NOT NULL REFERENCES broker_accounts(id),
    instrument_uid TEXT NOT NULL REFERENCES instruments(instrument_uid),
    active_snapshot_id UUID NOT NULL REFERENCES config_snapshots(id),
    enabled BOOLEAN NOT NULL DEFAULT false,
    sell_scope TEXT NOT NULL CHECK (sell_scope IN ('ladder_only', 'entire_position')),
    protected_base_qty BIGINT NOT NULL CHECK (protected_base_qty >= 0),
    max_reserve_nano BIGINT NOT NULL CHECK (max_reserve_nano >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, external_strategy_id)
);

CREATE TABLE ladder_levels (
    id UUID PRIMARY KEY,
    snapshot_id UUID NOT NULL REFERENCES config_snapshots(id),
    strategy_id UUID NOT NULL REFERENCES strategies(id),
    level_no BIGINT NOT NULL CHECK (level_no > 0),
    step_budget_nano BIGINT NOT NULL CHECK (step_budget_nano > 0),
    buy_price_nano BIGINT NOT NULL CHECK (buy_price_nano > 0),
    buy_lots BIGINT NOT NULL CHECK (buy_lots > 0),
    sell_price_nano BIGINT NOT NULL CHECK (sell_price_nano > 0),
    preview_sell_qty BIGINT NOT NULL CHECK (preview_sell_qty >= 0),
    entry_correction_nano BIGINT NOT NULL,
    exit_correction_nano BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (snapshot_id, strategy_id, level_no)
);

CREATE TABLE strategy_cycles (
    id UUID PRIMARY KEY,
    strategy_id UUID NOT NULL REFERENCES strategies(id),
    snapshot_id UUID NOT NULL REFERENCES config_snapshots(id),
    cycle_no BIGINT NOT NULL CHECK (cycle_no > 0),
    state TEXT NOT NULL CHECK (state IN (
        'DRAFT',
        'PUBLISHED',
        'RECONCILING',
        'BUY_ORDERS_ACTIVE',
        'POSITION_ACCUMULATING',
        'EXIT_PARTIALLY_FILLED',
        'CLOSING',
        'CLOSED',
        'PAUSED',
        'RECONCILIATION_REQUIRED',
        'ERROR'
    )),
    started_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    pause_reason TEXT,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (strategy_id, cycle_no)
);

CREATE UNIQUE INDEX strategy_cycles_one_active_idx
    ON strategy_cycles (strategy_id)
    WHERE state NOT IN ('CLOSED', 'ERROR');

CREATE TABLE orders (
    id UUID PRIMARY KEY,
    cycle_id UUID NOT NULL REFERENCES strategy_cycles(id),
    level_id UUID REFERENCES ladder_levels(id),
    side TEXT NOT NULL CHECK (side IN ('BUY', 'SELL')),
    command_revision BIGINT NOT NULL CHECK (command_revision > 0),
    idempotency_key UUID NOT NULL UNIQUE,
    broker_order_id TEXT,
    price_nano BIGINT NOT NULL CHECK (price_nano > 0),
    requested_lots BIGINT NOT NULL CHECK (requested_lots > 0),
    executed_lots BIGINT NOT NULL DEFAULT 0 CHECK (
        executed_lots >= 0 AND executed_lots <= requested_lots
    ),
    status TEXT NOT NULL CHECK (status IN (
        'PLANNED',
        'PERSISTED',
        'SUBMITTING',
        'NEW',
        'PARTIALLY_FILLED',
        'FILLED',
        'CANCELLING',
        'CANCELLED',
        'REPLACING',
        'REPLACED',
        'REJECTED',
        'UNKNOWN'
    )),
    submitted_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    raw_status TEXT,
    UNIQUE (broker_order_id)
);

CREATE UNIQUE INDEX orders_logical_command_idx
    ON orders (
        cycle_id,
        COALESCE(level_id, '00000000-0000-0000-0000-000000000000'::UUID),
        side,
        command_revision
    );

CREATE UNIQUE INDEX orders_one_active_sell_idx
    ON orders (cycle_id)
    WHERE side = 'SELL'
      AND status IN (
          'PLANNED',
          'PERSISTED',
          'SUBMITTING',
          'NEW',
          'PARTIALLY_FILLED',
          'CANCELLING',
          'REPLACING',
          'UNKNOWN'
      );

CREATE TABLE order_commands (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders(id),
    command_type TEXT NOT NULL CHECK (command_type IN ('place', 'replace', 'cancel')),
    idempotency_key UUID NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'persisted',
        'submitting',
        'acknowledged',
        'unknown',
        'failed'
    )),
    attempt_no BIGINT NOT NULL DEFAULT 1 CHECK (attempt_no > 0),
    request_metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    response_metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (idempotency_key, attempt_no)
);

CREATE TABLE executions (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders(id),
    broker_execution_id TEXT NOT NULL UNIQUE,
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    price_nano BIGINT NOT NULL CHECK (price_nano > 0),
    commission_nano BIGINT NOT NULL DEFAULT 0,
    executed_at TIMESTAMPTZ NOT NULL,
    raw_payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE position_snapshots (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES broker_accounts(id),
    instrument_uid TEXT NOT NULL REFERENCES instruments(instrument_uid),
    total_quantity BIGINT NOT NULL CHECK (total_quantity >= 0),
    free_quantity BIGINT NOT NULL CHECK (free_quantity >= 0),
    blocked_quantity BIGINT NOT NULL CHECK (blocked_quantity >= 0),
    average_price_nano BIGINT,
    source_at TIMESTAMPTZ NOT NULL,
    loaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (free_quantity + blocked_quantity <= total_quantity)
);

CREATE TABLE reconciliation_runs (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES broker_accounts(id),
    reason TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'matched', 'mismatch', 'failed')),
    mismatch_count BIGINT NOT NULL DEFAULT 0 CHECK (mismatch_count >= 0),
    details JSONB NOT NULL DEFAULT '{}'::JSONB,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);

CREATE TABLE audit_events (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    strategy_id UUID REFERENCES strategies(id),
    cycle_id UUID REFERENCES strategy_cycles(id),
    order_id UUID REFERENCES orders(id),
    actor TEXT NOT NULL CHECK (actor IN ('system', 'operator', 'config')),
    trace_id TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    topic TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    attempts BIGINT NOT NULL DEFAULT 0 CHECK (attempts >= 0)
);

CREATE INDEX orders_cycle_idx ON orders (cycle_id);
CREATE INDEX executions_order_idx ON executions (order_id);
CREATE INDEX position_snapshots_lookup_idx
    ON position_snapshots (account_id, instrument_uid, source_at DESC);
CREATE INDEX reconciliation_runs_account_idx
    ON reconciliation_runs (account_id, started_at DESC);
CREATE INDEX audit_events_trace_idx ON audit_events (trace_id);
CREATE INDEX outbox_events_unpublished_idx
    ON outbox_events (created_at)
    WHERE published_at IS NULL;
