package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func setupTestDir(t *testing.T) func() {
	origDir := dataDir
	testDir := filepath.Join(os.TempDir(), "qqgo_test_"+strconv.FormatInt(int64(os.Getpid()), 10))
	dataDir = testDir
	initDataDir()
	return func() {
		dataDir = origDir
		os.RemoveAll(testDir)
	}
}

func TestAppendPrivateLog(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	qq := int64(10001)
	targetQQ := int64(10002)

	appendPrivateLog(qq, targetQQ, "我", "hello")
	appendPrivateLog(qq, targetQQ, "Bob", "hi there")

	path := privateLogPath(qq, targetQQ)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Fatal("log file should not be empty")
	}

	lines := 0
	for _, line := range filepath.SplitList(content) {
		if line != "" {
			lines++
		}
	}
}

func TestAppendGroupLog(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	qq := int64(10001)
	groupID := "G12345"

	appendGroupLog(qq, groupID, "我", "group message")
	appendGroupLog(qq, groupID, "Alice", "reply")

	path := groupLogPath(qq, groupID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read group log file: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("group log file should not be empty")
	}
}

func TestPrivateLogWithZeroQQ(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	appendPrivateLog(0, 10002, "我", "should not be written")

	entries, _ := os.ReadDir(dataDir)
	if len(entries) != 0 {
		t.Fatalf("no directories should be created for QQ=0, got %d entries", len(entries))
	}
}

func TestSaveAndLoadToken(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	qq := int64(10001)
	token := "test_token_abc123"

	saveToken(qq, token)

	loaded, ok := loadToken(qq)
	if !ok {
		t.Fatal("token should exist")
	}
	if loaded != token {
		t.Fatalf("expected token %q, got %q", token, loaded)
	}
}

func TestLoadTokenNotFound(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	_, ok := loadToken(99999)
	if ok {
		t.Fatal("token should not exist for unknown QQ")
	}
}

func TestRemoveToken(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	qq := int64(10001)
	saveToken(qq, "token_to_remove")

	removeToken(qq)

	_, ok := loadToken(qq)
	if ok {
		t.Fatal("token should be removed")
	}
}

func TestSaveTokenWithZeroQQ(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	saveToken(0, "should_not_save")

	entries, _ := os.ReadDir(dataDir)
	if len(entries) != 0 {
		t.Fatalf("no directories should be created for QQ=0, got %d", len(entries))
	}
}

func TestSaveTokenWithEmptyToken(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	saveToken(10001, "")

	_, ok := loadToken(10001)
	if ok {
		t.Fatal("empty token should not be saved")
	}
}

func TestFindSavedQQ(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	qq, ok := findSavedQQ()
	if ok {
		t.Fatalf("no saved QQ expected, got %d", qq)
	}

	saveToken(10001, "token1")
	saveToken(10002, "token2")

	qq, ok = findSavedQQ()
	if !ok {
		t.Fatal("should find saved QQ")
	}
	if qq != 10001 && qq != 10002 {
		t.Fatalf("expected 10001 or 10002, got %d", qq)
	}
}

func TestTokenFilePermission(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	qq := int64(10001)
	saveToken(qq, "perm_test_token")

	path := filepath.Join(userDir(qq), "token")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("token file should exist: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Fatalf("expected permission 0600, got %o", perm)
	}
}

func TestLogDirectoryStructure(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	qq := int64(10001)
	appendPrivateLog(qq, 10002, "我", "test")
	appendGroupLog(qq, "G1", "我", "test")

	privateDir := filepath.Join(userDir(qq), "private")
	groupDir := filepath.Join(userDir(qq), "group")

	if _, err := os.Stat(privateDir); os.IsNotExist(err) {
		t.Fatal("private directory should exist")
	}
	if _, err := os.Stat(groupDir); os.IsNotExist(err) {
		t.Fatal("group directory should exist")
	}
}

func TestMultipleMessagesAppend(t *testing.T) {
	cleanup := setupTestDir(t)
	defer cleanup()

	qq := int64(10001)
	targetQQ := int64(10002)

	for i := 0; i < 10; i++ {
		appendPrivateLog(qq, targetQQ, "我", "message "+strconv.Itoa(i))
	}

	path := privateLogPath(qq, targetQQ)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}

	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 10 {
		t.Fatalf("expected 10 lines, got %d", lines)
	}
}
