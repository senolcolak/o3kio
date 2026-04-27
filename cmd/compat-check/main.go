package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cobaltcore-dev/o3k/internal/compat"
)

func main() {
	dir := flag.String("dir", ".", "Terraform directory to check")
	format := flag.String("output", "json", "Output format: json or text")
	flag.Parse()

	c := compat.NewChecker(compat.CheckerOptions{
		TerraformDir: *dir,
		OutputFormat: *format,
	})

	report, err := c.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "compat-check failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(report.String())
	if !report.Compatible {
		os.Exit(2)
	}
}
