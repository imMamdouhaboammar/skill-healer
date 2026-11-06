package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/imMamdouhaboammar/skill-healer/pkg/dashboard"
)

func runDashboard(args []string) {
	flags := flag.NewFlagSet("dashboard", flag.ExitOnError)
	port := flags.Int("port", 4317, "Local dashboard port")
	_ = flags.Parse(args)
	if err := dashboard.Serve(*port); err != nil {
		fmt.Fprintf(os.Stderr, "Dashboard error: %v\n", err)
		os.Exit(1)
	}
}
