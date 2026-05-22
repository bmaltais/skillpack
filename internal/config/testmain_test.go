package config_test

import (
	"os"
	"testing"

	"github.com/bmaltais/skillpack/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.RunWithTempHome(m)) }
