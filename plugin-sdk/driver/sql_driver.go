package driver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"google.golang.org/protobuf/types/known/emptypb"
)

// DriverName is the canonical name registered with database/sql.
const DriverName = "plugin-grpc"

var registered atomic.Bool

// Register installs a database/sql driver named DriverName backed by the
// provided SQLProxyClient. Subsequent calls reconfigure the underlying
// client without re-registering, so the SDK can call Register exactly once
// per process and still swap out the gRPC channel during reconnects.
//
// After Register, plugins may open a *sql.DB with sql.Open(DriverName, "").
// The DSN is ignored - the driver always proxies to the supplied client.
func Register(client pb.SQLProxyClient) {
	getActiveClient().set(client)
	if registered.CompareAndSwap(false, true) {
		sql.Register(DriverName, &grpcDriver{})
	}
}

// activeClient is the indirection that lets Register swap out the underlying
// gRPC client without re-registering with database/sql.
type activeClient struct {
	v atomic.Value
}

var globalActive activeClient

func getActiveClient() *activeClient { return &globalActive }

func (a *activeClient) set(c pb.SQLProxyClient) { a.v.Store(clientHolder{c}) }

func (a *activeClient) get() pb.SQLProxyClient {
	h, ok := a.v.Load().(clientHolder)
	if !ok {
		return nil
	}
	return h.c
}

// clientHolder wraps the interface value because atomic.Value requires a
// concrete type that never changes between Stores.
type clientHolder struct{ c pb.SQLProxyClient }

// grpcDriver is the database/sql driver entry point.
type grpcDriver struct{}

// Open returns a new connection. Because the underlying transport is gRPC,
// every "connection" shares the same channel; database/sql still pools the
// returned Conns and invokes Close when one is evicted.
func (d *grpcDriver) Open(_ string) (driver.Conn, error) {
	c := getActiveClient().get()
	if c == nil {
		return nil, fmt.Errorf("plugin-sdk: SQL driver used before Register")
	}
	return &grpcConn{client: c}, nil
}

// grpcConn implements driver.Conn plus the *Context interfaces so
// database/sql can pass a context.Context all the way down.
//
// When a transaction is active (activeTxID != ""), QueryContext and
// ExecContext route through TxQuery/TxExec instead of Query/Exec so
// that SQL issued via sql.Tx actually executes inside the server-side
// transaction.
type grpcConn struct {
	client     pb.SQLProxyClient
	closed     atomic.Bool
	activeTxID string // non-empty while a transaction is in progress
}

// Prepare returns a statement that defers its real work until Exec/Query.
// We do not have a server-side prepare, so the implementation simply
// remembers the query string.
func (c *grpcConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

// PrepareContext satisfies driver.ConnPrepareContext.
func (c *grpcConn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	return &grpcStmt{conn: c, query: query}, nil
}

// Close releases the local Conn handle. The shared gRPC channel is owned by
// the SDK, so we never tear it down here.
func (c *grpcConn) Close() error {
	c.closed.Store(true)
	return nil
}

// Begin is the legacy API; we always go through BeginTx.
func (c *grpcConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// BeginTx starts a transaction on the core via gRPC and returns a Tx that
// remembers the txID for subsequent TxQuery/TxExec calls. It also sets
// activeTxID on the conn so that QueryContext/ExecContext route through
// TxQuery/TxExec while the transaction is open.
func (c *grpcConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	if c.activeTxID != "" {
		return nil, fmt.Errorf("plugin-sdk: connection already has an active transaction")
	}
	resp, err := c.client.BeginTx(ctx, &pb.BeginTxRequest{
		IsolationLevel: isolationLevelString(sql.IsolationLevel(opts.Isolation)),
		ReadOnly:       opts.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	c.activeTxID = resp.GetTxId()
	return &grpcTx{conn: c, client: c.client, txID: c.activeTxID, ctx: ctx}, nil
}

// clearTx resets the transaction state on the connection. Called by grpcTx
// on Commit or Rollback to allow the conn to be reused for non-tx queries.
func (c *grpcConn) clearTx() { c.activeTxID = "" }

// Ping is a cheap RPC so database/sql can check liveness. We approximate it
// with a no-op SELECT 1 on the gRPC channel.
func (c *grpcConn) Ping(ctx context.Context) error {
	if c.closed.Load() {
		return driver.ErrBadConn
	}
	_, err := c.client.Query(ctx, &pb.SQLRequest{Query: "SELECT 1"})
	return err
}

// QueryContext satisfies driver.QueryerContext. When a transaction is active
// on this conn, the query is routed through TxQuery so it executes inside
// the server-side transaction.
func (c *grpcConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	pbArgs, err := namedValuesToProto(args)
	if err != nil {
		return nil, err
	}
	if c.activeTxID != "" {
		return c.txQuery(ctx, query, pbArgs)
	}
	resp, err := c.client.Query(ctx, &pb.SQLRequest{Query: query, Args: pbArgs})
	if err != nil {
		return nil, err
	}
	return newRows(resp), nil
}

// txQuery sends a query through the TxQuery RPC.
func (c *grpcConn) txQuery(ctx context.Context, query string, args []*pb.SQLValue) (driver.Rows, error) {
	req := &pb.TxSQLRequest{TxId: c.activeTxID, Query: query, Args: args}
	resp, err := c.client.TxQuery(ctx, req)
	if err != nil {
		return nil, err
	}
	return newRows(resp), nil
}

// ExecContext satisfies driver.ExecerContext. When a transaction is active
// on this conn, the statement is routed through TxExec so it executes
// inside the server-side transaction.
func (c *grpcConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	pbArgs, err := namedValuesToProto(args)
	if err != nil {
		return nil, err
	}
	if c.activeTxID != "" {
		return c.txExec(ctx, query, pbArgs)
	}
	resp, err := c.client.Exec(ctx, &pb.SQLRequest{Query: query, Args: pbArgs})
	if err != nil {
		return nil, err
	}
	return &execResult{rowsAffected: resp.GetRowsAffected(), lastID: resp.GetLastInsertId()}, nil
}

// txExec sends a statement through the TxExec RPC.
func (c *grpcConn) txExec(ctx context.Context, query string, args []*pb.SQLValue) (driver.Result, error) {
	req := &pb.TxSQLRequest{TxId: c.activeTxID, Query: query, Args: args}
	resp, err := c.client.TxExec(ctx, req)
	if err != nil {
		return nil, err
	}
	return &execResult{rowsAffected: resp.GetRowsAffected(), lastID: resp.GetLastInsertId()}, nil
}

// grpcStmt is a thin wrapper that re-routes Exec/Query through the conn.
type grpcStmt struct {
	conn  *grpcConn
	query string
}

func (s *grpcStmt) Close() error  { return nil }
func (s *grpcStmt) NumInput() int { return -1 }

func (s *grpcStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), valuesToNamed(args))
}
func (s *grpcStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), valuesToNamed(args))
}
func (s *grpcStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}
func (s *grpcStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

// grpcTx tracks the server-side transaction id and forwards the final
// commit/rollback back through the proxy. It holds a reference to the
// owning grpcConn so it can clear activeTxID on completion, and retains
// the context from BeginTx for Commit/Rollback RPCs.
type grpcTx struct {
	conn   *grpcConn
	client pb.SQLProxyClient
	txID   string
	ctx    context.Context
	done   atomic.Bool
}

func (t *grpcTx) Commit() error {
	if !t.done.CompareAndSwap(false, true) {
		return sql.ErrTxDone
	}
	t.conn.clearTx()
	_, err := t.client.CommitTx(t.ctx, &pb.TxIDRequest{TxId: t.txID})
	return err
}

func (t *grpcTx) Rollback() error {
	if !t.done.CompareAndSwap(false, true) {
		return sql.ErrTxDone
	}
	t.conn.clearTx()
	_, err := t.client.RollbackTx(t.ctx, &pb.TxIDRequest{TxId: t.txID})
	return err
}

// _ keeps the emptypb import alive - CommitTx/RollbackTx return *emptypb.Empty
// values that we discard, but referencing the package once here makes the
// dependency explicit even if the compiler optimises the discard away.
var _ = (*emptypb.Empty)(nil)

// execResult is a trivial driver.Result implementation.
type execResult struct {
	rowsAffected int64
	lastID       int64
}

func (r *execResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r *execResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

// grpcRows iterates over the SQLResponse returned by Query. The full result
// set is materialised by the proxy; we just hand rows back one at a time.
type grpcRows struct {
	columns []string
	rows    []*pb.SQLRow
	idx     int
}

func newRows(resp *pb.SQLResponse) *grpcRows {
	return &grpcRows{columns: resp.GetColumns(), rows: resp.GetRows()}
}

func (r *grpcRows) Columns() []string { return r.columns }
func (r *grpcRows) Close() error      { return nil }

func (r *grpcRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.idx]
	r.idx++
	values := row.GetValues()
	if len(values) != len(dest) {
		return fmt.Errorf("plugin-sdk: column count mismatch: got %d, want %d", len(values), len(dest))
	}
	for i, v := range values {
		dest[i] = sqlValueToDriver(v)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Value conversion helpers
// ---------------------------------------------------------------------------

func namedValuesToProto(args []driver.NamedValue) ([]*pb.SQLValue, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make([]*pb.SQLValue, 0, len(args))
	for _, a := range args {
		v, err := goValueToProto(a.Value)
		if err != nil {
			return nil, fmt.Errorf("plugin-sdk: arg %d: %w", a.Ordinal, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func valuesToNamed(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, v := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return out
}

func goValueToProto(v interface{}) (*pb.SQLValue, error) {
	if v == nil {
		return &pb.SQLValue{Value: &pb.SQLValue_Null{Null: true}}, nil
	}
	switch x := v.(type) {
	case bool:
		return &pb.SQLValue{Value: &pb.SQLValue_BoolValue{BoolValue: x}}, nil
	case int:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case int8:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case int16:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case int32:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case int64:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: x}}, nil
	case uint8:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case uint16:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case uint32:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case uint64:
		return &pb.SQLValue{Value: &pb.SQLValue_IntValue{IntValue: int64(x)}}, nil
	case float32:
		return &pb.SQLValue{Value: &pb.SQLValue_FloatValue{FloatValue: float64(x)}}, nil
	case float64:
		return &pb.SQLValue{Value: &pb.SQLValue_FloatValue{FloatValue: x}}, nil
	case string:
		return &pb.SQLValue{Value: &pb.SQLValue_StringValue{StringValue: x}}, nil
	case []byte:
		return &pb.SQLValue{Value: &pb.SQLValue_BytesValue{BytesValue: x}}, nil
	case time.Time:
		return &pb.SQLValue{Value: &pb.SQLValue_StringValue{StringValue: x.Format(time.RFC3339Nano)}}, nil
	default:
		return nil, fmt.Errorf("unsupported argument type %T", v)
	}
}

func sqlValueToDriver(v *pb.SQLValue) driver.Value {
	if v == nil {
		return nil
	}
	switch x := v.GetValue().(type) {
	case *pb.SQLValue_Null:
		return nil
	case *pb.SQLValue_BoolValue:
		return x.BoolValue
	case *pb.SQLValue_IntValue:
		return x.IntValue
	case *pb.SQLValue_FloatValue:
		return x.FloatValue
	case *pb.SQLValue_StringValue:
		return x.StringValue
	case *pb.SQLValue_BytesValue:
		out := make([]byte, len(x.BytesValue))
		copy(out, x.BytesValue)
		return out
	default:
		return nil
	}
}

func isolationLevelString(level sql.IsolationLevel) string {
	switch level {
	case sql.LevelDefault:
		return ""
	case sql.LevelReadCommitted:
		return "read_committed"
	case sql.LevelRepeatableRead:
		return "repeatable_read"
	case sql.LevelSerializable:
		return "serializable"
	default:
		return ""
	}
}

// Compile-time interface assertions.
var (
	_ driver.Driver             = (*grpcDriver)(nil)
	_ driver.Conn               = (*grpcConn)(nil)
	_ driver.ConnBeginTx        = (*grpcConn)(nil)
	_ driver.ConnPrepareContext = (*grpcConn)(nil)
	_ driver.QueryerContext     = (*grpcConn)(nil)
	_ driver.ExecerContext      = (*grpcConn)(nil)
	_ driver.Pinger             = (*grpcConn)(nil)
	_ driver.Stmt               = (*grpcStmt)(nil)
	_ driver.StmtExecContext    = (*grpcStmt)(nil)
	_ driver.StmtQueryContext   = (*grpcStmt)(nil)
	_ driver.Tx                 = (*grpcTx)(nil)
	_ driver.Rows               = (*grpcRows)(nil)
	_ driver.Result             = (*execResult)(nil)
)
