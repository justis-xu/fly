package sub

import (
	"crypto/sha256"
	"encoding/json"
	"fly/internal/storage"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type cacheRecord struct {
	Source string `json:"source_hash"`
	Body   []byte `json:"body"`
	Info   *Info  `json:"info"`
}

func sourceHash(source string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(source))) }
func cachePath(dir, source string) string {
	return filepath.Join(dir, "subscriptions", sourceHash(source)+".json")
}
func SaveSourceCache(dir, source string, body []byte, info *Info) error {
	data, err := json.Marshal(cacheRecord{sourceHash(source), body, info})
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(dir, "subscriptions"), 0700); err != nil {
		return err
	}
	return storage.WriteFile(cachePath(dir, source), data)
}
func LoadSourceCache(dir, source string) ([]byte, *Info, bool) {
	b, err := os.ReadFile(cachePath(dir, source))
	if os.IsNotExist(err) {
		b, err = os.ReadFile(filepath.Join(dir, "subscription.json"))
	} // first source-bound implementation
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
func HasSourceCaches(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "subscription.json")); err == nil {
		return true
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "subscriptions"))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			return true
		}
	}
	return false
}

// RemoveSourceCache is used only to roll back a newly created transaction-owned record.
func RemoveSourceCache(dir, source string) error {
	err := os.Remove(cachePath(dir, source))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
