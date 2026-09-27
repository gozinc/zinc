// Command zincbench records head-to-head benchmark runs and builds a local
// dashboard that compares any two of them.
//
// Every run is saved as an immutable record in benchmarks/results/ (ignored by
// git): the raw samples, the git state including uncommitted changes, and the
// environment. The dashboard compares the latest run with a pinned baseline by
// default, or with the previous run.
//
//	go run ./cmd/zincbench record -note "router: cache promotion at 8"
//	go run ./cmd/zincbench dash -open
//	go run ./cmd/zincbench baseline <run-id>
//	go run ./cmd/zincbench list
//
// Run the commands from the benchmarks module, or use the Makefile targets in
// the repository root.
package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `zincbench records benchmark runs and compares them.

Usage:
  zincbench record   [-release version] [-note text] [-count 10] [-benchtime 100ms] [-bench regexp] [-baseline]
                     [-zinc-only [-rivals run-id]] [-micro=false]
  zincbench compare  [-base run-id] [-slow-pct 5] [-noisy-pct 12] [-slow-ns 15] [-all] [run-id | latest]
  zincbench ab       -scenarios name1,name2 [-base commit] [-rounds 6] [-count 3] [-benchtime 100ms]
  zincbench profile  [-benchtime 3s] [-top 20] scenario
  zincbench import   -log file[.gz] [-release version] [-commit sha] [-date RFC3339] [-note text] [-baseline]
  zincbench import-zinc -logs file1,file2 -rivals run-id -commit sha -date RFC3339 [-release version] [-note text]
  zincbench bundle   -release version [-o file.zip] [-include path1,path2]
  zincbench baseline [run-id | latest]
  zincbench list
  zincbench dash     [-o file] [-open]

Records and the dashboard live in benchmarks/results/, which git ignores.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	store, err := openStore()
	if err != nil {
		fatal(err)
	}

	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "record":
		err = cmdRecord(store, args)
	case "import":
		err = cmdImport(store, args)
	case "import-zinc":
		err = cmdImportZinc(store, args)
	case "bundle":
		err = cmdBundle(store, args)
	case "baseline":
		err = cmdBaseline(store, args)
	case "ab":
		err = cmdAB(store, args)
	case "compare":
		err = cmdCompare(store, args)
	case "profile":
		err = cmdProfile(store, args)
	case "list":
		err = cmdList(store)
	case "dash":
		err = cmdDash(store, args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "zincbench: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "zincbench:", err)
	os.Exit(1)
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	return fs
}
