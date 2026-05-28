package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/qqgo/server/internal/model"
)

const defaultDataDir = "DATA"

var dataDir = defaultDataDir

func initDataDir() {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Printf("[localstore] failed to create DATA dir: %v", err)
	}
}

func userDir(qq int64) string {
	return filepath.Join(dataDir, strconv.FormatInt(qq, 10))
}

func ensureUserDir(qq int64) {
	dir := userDir(qq)
	os.MkdirAll(filepath.Join(dir, "private"), 0755)
	os.MkdirAll(filepath.Join(dir, "group"), 0755)
}

func privateLogPath(myQQ int64, targetQQ int64) string {
	return filepath.Join(userDir(myQQ), "private", strconv.FormatInt(targetQQ, 10)+".log")
}

func groupLogPath(myQQ int64, groupID string) string {
	safeID := strings.ReplaceAll(groupID, "/", "_")
	return filepath.Join(userDir(myQQ), "group", safeID+".log")
}

func appendLog(path string, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[localstore] open log file error: %v", err)
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

func appendPrivateLog(myQQ int64, targetQQ int64, direction string, content string) {
	if myQQ == 0 {
		return
	}
	ensureUserDir(myQQ)
	path := privateLogPath(myQQ, targetQQ)
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] [%s] %s", timeStr, direction, content)
	appendLog(path, line)
}

func appendGroupLog(myQQ int64, groupID string, sender string, content string) {
	if myQQ == 0 {
		return
	}
	ensureUserDir(myQQ)
	path := groupLogPath(myQQ, groupID)
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] [%s] %s", timeStr, sender, content)
	appendLog(path, line)
}

func saveToken(qq int64, token string) {
	if qq == 0 || token == "" {
		return
	}
	ensureUserDir(qq)
	path := filepath.Join(userDir(qq), "token")
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		log.Printf("[localstore] save token error: %v", err)
	}
}

func loadToken(qq int64) (string, bool) {
	path := filepath.Join(userDir(qq), "token")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", false
	}
	return token, true
}

func saveAccessToken(qq int64, token string) {
	if qq == 0 || token == "" {
		return
	}
	ensureUserDir(qq)
	path := filepath.Join(userDir(qq), "access_token")
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		log.Printf("[localstore] save access_token error: %v", err)
	}
}

func loadAccessToken(qq int64) (string, bool) {
	path := filepath.Join(userDir(qq), "access_token")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", false
	}
	return token, true
}

func saveRefreshToken(qq int64, token string) {
	if qq == 0 || token == "" {
		return
	}
	ensureUserDir(qq)
	path := filepath.Join(userDir(qq), "refresh_token")
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		log.Printf("[localstore] save refresh_token error: %v", err)
	}
}

func loadRefreshToken(qq int64) (string, bool) {
	path := filepath.Join(userDir(qq), "refresh_token")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", false
	}
	return token, true
}

func removeToken(qq int64) {
	if qq == 0 {
		return
	}
	os.Remove(filepath.Join(userDir(qq), "token"))
	os.Remove(filepath.Join(userDir(qq), "access_token"))
	os.Remove(filepath.Join(userDir(qq), "refresh_token"))
}

func findSavedQQ() (int64, bool) {
	initDataDir()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return 0, false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		qq, err := strconv.ParseInt(entry.Name(), 10, 64)
		if err != nil {
			continue
		}
		accessTokenPath := filepath.Join(dataDir, entry.Name(), "access_token")
		if _, err := os.Stat(accessTokenPath); err == nil {
			return qq, true
		}
		tokenPath := filepath.Join(dataDir, entry.Name(), "token")
		if _, err := os.Stat(tokenPath); err == nil {
			return qq, true
		}
	}
	return 0, false
}

func saveReceivedFile(myQQ int64, fromQQ int64, fc model.FileContent) string {
	if myQQ == 0 {
		return ""
	}
	ensureUserDir(myQQ)
	recvDir := filepath.Join(userDir(myQQ), "recv")
	os.MkdirAll(recvDir, 0755)

	safeName := strconv.FormatInt(fromQQ, 10) + "_" + filepath.Base(fc.Filename)
	savePath := filepath.Join(recvDir, safeName)

	err := os.WriteFile(savePath, []byte(fc.Data), 0644)
	if err != nil {
		log.Printf("[localstore] save received file error: %v", err)
		return ""
	}
	return savePath
}
