package e2e

import (
	"flag"
	"fmt"
	"os"
	"testing"

	_ "github.com/onsi/ginkgo/v2"

	"monitoragent/pkg/util/log"
	// test source
	"monitoragent/test/e2e/framework"
	_ "monitoragent/test/e2e/legacy/basic"
	_ "monitoragent/test/e2e/legacy/features"
	_ "monitoragent/test/e2e/legacy/plugin"
	_ "monitoragent/test/e2e/v1/basic"
	_ "monitoragent/test/e2e/v1/features"
	_ "monitoragent/test/e2e/v1/plugin"
)

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	framework.RegisterCommonFlags(flag.CommandLine)
	flag.Parse()
}

func TestMain(m *testing.M) {
	// Register test flags, then parse flags.
	handleFlags()

	if err := framework.ValidateTestContext(&framework.TestContext); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	log.InitLog("console", framework.TestContext.LogLevel, 0, true)
	os.Exit(m.Run())
}

func TestE2E(t *testing.T) {
	RunE2ETests(t)
}
