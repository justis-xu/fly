package sub

import (
	"crypto/sha256"
	"encoding/json"
	"fly/internal/storage"
	"fmt"
	"os"
	"path/filepath"
)

type cacheRecord struct {
	Source string `json:"source_hash"`
	Body   []byte `json:"body"`
	Info   *Info  `json:"info"`
}

func sourceHash(source string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(source))) }
func SaveSourceCache(dir, source string, body []byte, info *Info) error {
	data, err := json.Marshal(cacheRecord{sourceHash(source), body, info})
	if err != nil {
		return err
	}
	return storage.WriteFile(filepath.Join(dir, "subscription.json"), data)
}
func LoadSourceCache(dir, source string) ([]byte, *Info, bool) {
	b, err := os.ReadFile(filepath.Join(dir, "subscription.json"))
	if err != nil {
		return nil, nil, false
	}
	var rec cacheRecord
	if json.Unmarshal(b, &rec) != nil || rec.Source != sourceHash(source) || len(rec.Body) == 0 || len(rec.Body) > MaxBody {
		return nil, nil, false
	}
	if rec.Info == nil {
		rec.Info = &Info{}
	}
	return rec.Body, rec.Info, true
}
