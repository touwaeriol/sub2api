package tokenutil

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"

	_ "embed"
)

const cl100kBaseEncodingURL = "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"

//go:embed data/cl100k_base.tiktoken
var cl100kBaseEncodingData []byte

type embeddedBpeLoader struct {
	fallback tiktoken.BpeLoader
}

func (l *embeddedBpeLoader) LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	if tiktokenBpeFile == cl100kBaseEncodingURL {
		return loadTiktokenBpeFromBytes(cl100kBaseEncodingData)
	}
	if l.fallback != nil {
		return l.fallback.LoadTiktokenBpe(tiktokenBpeFile)
	}
	return nil, fmt.Errorf("unsupported tiktoken bpe file: %s", tiktokenBpeFile)
}

func loadTiktokenBpeFromBytes(contents []byte) (map[string]int, error) {
	bpeRanks := make(map[string]int)

	for _, line := range strings.Split(string(bytes.TrimSpace(contents)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tiktoken bpe line: %q", line)
		}

		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, err
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		bpeRanks[string(token)] = rank
	}

	return bpeRanks, nil
}

func init() {
	// 避免运行时拉取 openaipublic 的 encoding 文件：对 cl100k_base 使用内置数据。
	tiktoken.SetBpeLoader(&embeddedBpeLoader{fallback: tiktoken.NewDefaultBpeLoader()})
}

var (
	cl100kOnce sync.Once
	cl100kEnc  *tiktoken.Tiktoken
	cl100kErr  error
)

func getCl100kEncoding() (*tiktoken.Tiktoken, error) {
	cl100kOnce.Do(func() {
		cl100kEnc, cl100kErr = tiktoken.GetEncoding("cl100k_base")
	})
	return cl100kEnc, cl100kErr
}

// CountTokensForText 使用分词库计算 token 数量（当前使用 cl100k_base）。
//
// 返回 (tokens, true) 表示分词成功；
// 返回 (0, false) 表示分词库初始化失败（调用方可自行 fallback）。
func CountTokensForText(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, true
	}

	enc, err := getCl100kEncoding()
	if err != nil || enc == nil {
		return 0, false
	}

	return len(enc.Encode(s, nil, nil)), true
}
