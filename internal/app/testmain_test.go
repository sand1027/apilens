package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("APILENS_HISTORY_POINTER", filepath.Join(os.TempDir(), fmt.Sprintf("apilens-test-ptr-%d", os.Getpid())))
	os.Exit(m.Run())
}
