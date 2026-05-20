package plugin

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// txAutoRollbackTimeout is the auto-rollback timeout preventing plugins from
// forgetting to Commit/Rollback, which would hold a DB connection indefinitely.
const txAutoRollbackTimeout = 30 * time.Second

// txCleanupInterval is the polling interval for the transaction cleanup goroutine.
const txCleanupInterval = 10 * time.Second

// SDKServer implements SQLProxy / RedisProxy gRPC services and exposes the
// EventBus service via the eventBusAdapter wrapper.
//
// Security isolation:
//   - SQL: SQLTableGate restricts each plugin to its own tables (plugin_<name>_*).
//   - Redis: RedisKeyPrefixer auto-prefixes keys/channels per plugin.
//   - Plugin identity flows via gRPC metadata (x-plugin-name), extracted by the SDK interceptor.
type SDKServer struct {
	pluginsdk.UnimplementedSQLProxyServer
	pluginsdk.UnimplementedRedisProxyServer

	db    *sql.DB
	redis *redis.Client

	txMu sync.Mutex
	txs  map[string]*activeTx

	sqlGate *SQLTableGate

	stop context.CancelFunc
}

type activeTx struct {
	tx        *sql.Tx
	startedAt time.Time
	pluginID  string
}

// NewSDKServer creates an SDK service instance and starts the tx cleanup goroutine.
func NewSDKServer(db *sql.DB, rdb *redis.Client) *SDKServer {
	s := &SDKServer{
		db:      db,
		redis:   rdb,
		txs:     make(map[string]*activeTx),
		sqlGate: NewSQLTableGate(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.stop = cancel
	go s.cleanupLoop(ctx)
	return s
}

// SQLGate returns the SQL table gate for registering allowed tables during migration.
func (s *SDKServer) SQLGate() *SQLTableGate { return s.sqlGate }

// redisPrefixer returns a RedisKeyPrefixer for the calling plugin.
func (s *SDKServer) redisPrefixer(ctx context.Context) (*RedisKeyPrefixer, error) {
	name, err := pluginNameFromContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "plugin identity: %v", err)
	}
	return NewRedisKeyPrefixer(name), nil
}

// Stop cancels background goroutines and rolls back all pending transactions.
func (s *SDKServer) Stop() {
	if s.stop != nil {
		s.stop()
	}
	s.txMu.Lock()
	defer s.txMu.Unlock()
	for id, t := range s.txs {
		_ = t.tx.Rollback()
		delete(s.txs, id)
	}
}

// RegisterServices registers all SDK services onto the grpcServer.
func (s *SDKServer) RegisterServices(grpcServer *grpc.Server) {
	pluginsdk.RegisterSQLProxyServer(grpcServer, s)
	pluginsdk.RegisterRedisProxyServer(grpcServer, s)
	pluginsdk.RegisterEventBusServer(grpcServer, &eventBusAdapter{srv: s})
}

// GRPCServerOptions returns the interceptors that enforce per-plugin isolation.
func GRPCServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(sdkUnaryInterceptor),
		grpc.StreamInterceptor(sdkStreamInterceptor),
	}
}

func (s *SDKServer) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(txCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.rollbackTimedOutTx()
		}
	}
}

func (s *SDKServer) rollbackTimedOutTx() {
	cutoff := time.Now().Add(-txAutoRollbackTimeout)

	s.txMu.Lock()
	expired := make([]string, 0)
	for id, t := range s.txs {
		if t.startedAt.Before(cutoff) {
			expired = append(expired, id)
		}
	}
	s.txMu.Unlock()

	for _, id := range expired {
		s.txMu.Lock()
		t, ok := s.txs[id]
		if ok {
			delete(s.txs, id)
		}
		s.txMu.Unlock()
		if !ok {
			continue
		}
		if err := t.tx.Rollback(); err != nil {
			slog.Warn("rollback timed-out plugin tx", "tx_id", id, "error", err)
		} else {
			slog.Warn("rolled back timed-out plugin tx", "tx_id", id, "age", time.Since(t.startedAt).String())
		}
	}
}

// ============================================================
// SQLProxy implementation
// ============================================================

func (s *SDKServer) Query(ctx context.Context, req *pluginsdk.SQLRequest) (*pluginsdk.SQLResponse, error) {
	if s.db == nil {
		return nil, status.Error(codes.FailedPrecondition, "sql db not configured")
	}
	if err := s.checkSQLAccess(ctx, req.GetQuery()); err != nil {
		return nil, err
	}
	args := convertSQLValues(req.GetArgs())
	rows, err := s.db.QueryContext(ctx, req.GetQuery(), args...)
	if err != nil {
		return nil, toGRPCError("plugin sql query", err)
	}
	defer rows.Close()
	return scanRowsToResponse(rows)
}

func (s *SDKServer) Exec(ctx context.Context, req *pluginsdk.SQLRequest) (*pluginsdk.ExecResponse, error) {
	if s.db == nil {
		return nil, status.Error(codes.FailedPrecondition, "sql db not configured")
	}
	if err := s.checkSQLAccess(ctx, req.GetQuery()); err != nil {
		return nil, err
	}
	args := convertSQLValues(req.GetArgs())
	res, err := s.db.ExecContext(ctx, req.GetQuery(), args...)
	if err != nil {
		return nil, toGRPCError("plugin sql exec", err)
	}
	return execResultToResponse(res), nil
}

func (s *SDKServer) BeginTx(ctx context.Context, req *pluginsdk.BeginTxRequest) (*pluginsdk.TxResponse, error) {
	if s.db == nil {
		return nil, status.Error(codes.FailedPrecondition, "sql db not configured")
	}
	opts, err := parseTxOptions(req)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, toGRPCError("plugin begin tx", err)
	}
	id := uuid.NewString()
	s.txMu.Lock()
	s.txs[id] = &activeTx{tx: tx, startedAt: time.Now()}
	s.txMu.Unlock()
	return &pluginsdk.TxResponse{TxId: id}, nil
}

// parseTxOptions parses isolation level; invalid levels return InvalidArgument.
func parseTxOptions(req *pluginsdk.BeginTxRequest) (*sql.TxOptions, error) {
	opts := &sql.TxOptions{ReadOnly: req.GetReadOnly()}
	switch req.GetIsolationLevel() {
	case "read_committed":
		opts.Isolation = sql.LevelReadCommitted
	case "repeatable_read":
		opts.Isolation = sql.LevelRepeatableRead
	case "serializable":
		opts.Isolation = sql.LevelSerializable
	case "":
		// use driver default
	default:
		return nil, status.Errorf(codes.InvalidArgument,
			"unsupported isolation level: %s", req.GetIsolationLevel())
	}
	return opts, nil
}

func (s *SDKServer) TxQuery(ctx context.Context, req *pluginsdk.TxSQLRequest) (*pluginsdk.SQLResponse, error) {
	tx, err := s.lookupTx(req.GetTxId())
	if err != nil {
		return nil, err
	}
	if err := s.checkSQLAccess(ctx, req.GetQuery()); err != nil {
		return nil, err
	}
	args := convertSQLValues(req.GetArgs())
	rows, err := tx.QueryContext(ctx, req.GetQuery(), args...)
	if err != nil {
		return nil, toGRPCError("plugin tx query", err)
	}
	defer rows.Close()
	return scanRowsToResponse(rows)
}

func (s *SDKServer) TxExec(ctx context.Context, req *pluginsdk.TxSQLRequest) (*pluginsdk.ExecResponse, error) {
	tx, err := s.lookupTx(req.GetTxId())
	if err != nil {
		return nil, err
	}
	if err := s.checkSQLAccess(ctx, req.GetQuery()); err != nil {
		return nil, err
	}
	args := convertSQLValues(req.GetArgs())
	res, err := tx.ExecContext(ctx, req.GetQuery(), args...)
	if err != nil {
		return nil, toGRPCError("plugin tx exec", err)
	}
	return execResultToResponse(res), nil
}

func (s *SDKServer) CommitTx(_ context.Context, req *pluginsdk.TxIDRequest) (*emptypb.Empty, error) {
	tx, err := s.popTx(req.GetTxId())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, toGRPCError("plugin commit tx", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) RollbackTx(_ context.Context, req *pluginsdk.TxIDRequest) (*emptypb.Empty, error) {
	tx, err := s.popTx(req.GetTxId())
	if err != nil {
		return nil, err
	}
	if err := tx.Rollback(); err != nil {
		return nil, toGRPCError("plugin rollback tx", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) lookupTx(id string) (*sql.Tx, error) {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	t, ok := s.txs[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown tx_id: %s", id)
	}
	return t.tx, nil
}

func (s *SDKServer) popTx(id string) (*sql.Tx, error) {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	t, ok := s.txs[id]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "unknown tx_id: %s", id)
	}
	delete(s.txs, id)
	return t.tx, nil
}

// checkSQLAccess validates plugin identity and table access.
func (s *SDKServer) checkSQLAccess(ctx context.Context, query string) error {
	name, err := pluginNameFromContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "plugin identity: %v", err)
	}
	if err := s.sqlGate.CheckQuery(name, query); err != nil {
		return status.Errorf(codes.PermissionDenied, "%v", err)
	}
	return nil
}

// ============================================================
// SQL type conversion helpers
// ============================================================

func convertSQLValues(values []*pluginsdk.SQLValue) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = sqlValueToInterface(v)
	}
	return out
}

func sqlValueToInterface(v *pluginsdk.SQLValue) any {
	if v == nil {
		return nil
	}
	switch payload := v.GetValue().(type) {
	case *pluginsdk.SQLValue_Null:
		return nil
	case *pluginsdk.SQLValue_IntValue:
		return payload.IntValue
	case *pluginsdk.SQLValue_FloatValue:
		return payload.FloatValue
	case *pluginsdk.SQLValue_StringValue:
		return payload.StringValue
	case *pluginsdk.SQLValue_BytesValue:
		return payload.BytesValue
	case *pluginsdk.SQLValue_BoolValue:
		return payload.BoolValue
	default:
		return nil
	}
}

func interfaceToSQLValue(v any) *pluginsdk.SQLValue {
	if v == nil {
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_Null{Null: true}}
	}
	switch x := v.(type) {
	case bool:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_BoolValue{BoolValue: x}}
	case int:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_IntValue{IntValue: int64(x)}}
	case int32:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_IntValue{IntValue: int64(x)}}
	case int64:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_IntValue{IntValue: x}}
	case float32:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_FloatValue{FloatValue: float64(x)}}
	case float64:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_FloatValue{FloatValue: x}}
	case string:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_StringValue{StringValue: x}}
	case []byte:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_BytesValue{BytesValue: x}}
	case time.Time:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_StringValue{StringValue: x.Format(time.RFC3339Nano)}}
	default:
		return &pluginsdk.SQLValue{Value: &pluginsdk.SQLValue_StringValue{StringValue: fmt.Sprintf("%v", x)}}
	}
}

func scanRowsToResponse(rows *sql.Rows) (*pluginsdk.SQLResponse, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, toGRPCError("plugin sql columns", err)
	}

	resp := &pluginsdk.SQLResponse{Columns: cols}
	for rows.Next() {
		holders := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, toGRPCError("plugin sql scan", err)
		}
		row := &pluginsdk.SQLRow{Values: make([]*pluginsdk.SQLValue, len(cols))}
		for i, v := range holders {
			row.Values[i] = interfaceToSQLValue(v)
		}
		resp.Rows = append(resp.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, toGRPCError("plugin sql iterate", err)
	}
	return resp, nil
}

func execResultToResponse(res sql.Result) *pluginsdk.ExecResponse {
	out := &pluginsdk.ExecResponse{}
	if rows, err := res.RowsAffected(); err == nil {
		out.RowsAffected = rows
	}
	if id, err := res.LastInsertId(); err == nil {
		out.LastInsertId = id
	}
	return out
}