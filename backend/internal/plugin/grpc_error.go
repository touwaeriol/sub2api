package plugin

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toGRPCError 将 Go 原生错误转换为语义化的 gRPC status error。
// 调用方传入操作描述(如 "plugin sql query")和原始 error,
// 返回值可直接作为 gRPC handler 的 error 返回。
func toGRPCError(op string, err error) error {
	if err == nil {
		return nil
	}
	code := classifyError(err)
	return status.Errorf(code, "%s: %v", op, err)
}

// classifyError 根据错误类型推断最合适的 gRPC 状态码。
func classifyError(err error) codes.Code {
	if err == nil {
		return codes.OK
	}

	// 找不到数据
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, redis.Nil) {
		return codes.NotFound
	}

	// 超时 / 取消
	if errors.Is(err, context.DeadlineExceeded) {
		return codes.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return codes.Canceled
	}

	// 连接类错误(Redis)
	if isConnectionError(err) {
		return codes.Unavailable
	}

	// SQL 语法 / 约束违反 → 客户端发来的 query 有问题
	if isSQLClientError(err) {
		return codes.InvalidArgument
	}

	// 权限被 gate 拒绝(包含 "denied access" 关键字)
	if isPermissionDenied(err) {
		return codes.PermissionDenied
	}

	return codes.Internal
}

// isConnectionError 检测是否为连接/网络类错误。
func isConnectionError(err error) bool {
	msg := err.Error()
	patterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"i/o timeout",
	}
	for _, p := range patterns {
		if strings.Contains(strings.ToLower(msg), p) {
			return true
		}
	}
	return false
}

// isSQLClientError 检测是否为 SQL 语法/约束错误(客户端问题)。
func isSQLClientError(err error) bool {
	msg := strings.ToLower(err.Error())
	patterns := []string{
		"syntax error",
		"duplicate key",
		"unique constraint",
		"foreign key constraint",
		"check constraint",
		"not-null constraint",
		"violates",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// isPermissionDenied 检测是否为权限拒绝错误(SQLTableGate 等)。
func isPermissionDenied(err error) bool {
	return strings.Contains(err.Error(), "denied access")
}
