package antigravity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"strconv"
)

// DeriveSessionID 基于原始 sessionId 和账号 ID 派生新的 sessionId
// 确定性：相同输入总是产生相同输出
// 格式：负号 + 19位十进制数字（如 -4611686018427387903）
func DeriveSessionID(originalSessionID string, accountID int64) string {
	if originalSessionID == "" {
		return ""
	}
	combined := originalSessionID + ":" + strconv.FormatInt(accountID, 10)
	h := sha256.Sum256([]byte(combined))
	n := int64(binary.BigEndian.Uint64(h[:8])) & 0x7FFFFFFFFFFFFFFF
	return "-" + strconv.FormatInt(n, 10)
}

// ReplaceSessionIDForOAuth 替换 v1internal 请求体中的 sessionId（仅 OAuth 账号调用）
// 请求体结构: {"request": {"sessionId": "...", ...}, ...}
// 返回替换后的请求体，原始请求体不变
func ReplaceSessionIDForOAuth(body []byte, accountID int64) ([]byte, error) {
	var outer map[string]any
	if err := json.Unmarshal(body, &outer); err != nil {
		return body, err
	}

	inner, ok := outer["request"].(map[string]any)
	if !ok {
		return body, nil
	}

	originalSessionID, _ := inner["sessionId"].(string)
	if originalSessionID == "" {
		return body, nil
	}

	inner["sessionId"] = DeriveSessionID(originalSessionID, accountID)
	return json.Marshal(outer)
}
