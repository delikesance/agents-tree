package main

import (
	"os"

	"github.com/delikesance/agents-tree/internal/baseline"
)

func main() {
	r, err := baseline.Analyze(baseline.Options{Session: os.Args[1], ProjectsDir: "/root/.claude/projects", Home: "/root"})
	if err != nil {
		panic(err)
	}
	r.Write(os.Stdout)
}
