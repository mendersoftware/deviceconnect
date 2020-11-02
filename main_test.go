package main

import (
	"flag"
	"os"
	"testing"
)

var (
	acceptanceTesting bool
)

func init() {
	flag.BoolVar(&acceptanceTesting, "acceptance-testing", false,
		"Acceptance testing mode, starts the application main function "+
			"with cover mode enabled. Non-flag arguments are passed"+
			"to the main application, add '--' after test flags to"+
			"pass flags to main.",
	)
}

func TestMain(m *testing.M) {
	flag.Parse()
	if acceptanceTesting {
		// Override 'run' flags to only execute TestDoMain
		flag.Set("test.run", "TestDoMain")
	}
	os.Exit(m.Run())
}

func TestDoMain(t *testing.T) {
	if !acceptanceTesting {
		t.Skip()
	}
	doMain(append(os.Args[:1], flag.Args()...))
}
