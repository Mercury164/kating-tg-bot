package util

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "strings"
    "time"
)

func NowISO() string {
    return time.Now().Format(time.RFC3339)
}

func NormalizeBoolRU(s string) bool {
    s = strings.TrimSpace(strings.ToLower(s))
    switch s {
    case "да", "yes", "true", "1", "y":
        return true
    default:
        return false
    }
}

func HMACSHA256Hex(secret, msg string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(msg))
    return hex.EncodeToString(mac.Sum(nil))
}
