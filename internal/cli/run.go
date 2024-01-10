package cli

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&cssPath, "css", "static/css/tailwind.css", "CSS file path used by tailwind")
}

var (
	cssPath string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Zinc app",
	Args:  cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		airRun := exec.CommandContext(ctx, "air", "run")
		setInOuts(airRun)

		err := airRun.Start()
		if err != nil {
			logErrorAndExit(err)
		}

		tailwindWatch := exec.CommandContext(ctx, "tailwindcss", "build", "-o", cssPath, "--watch")
		setInOuts(tailwindWatch)

		err = tailwindWatch.Start()
		if err != nil {
			logErrorAndExit(err)
		}

		templGen := exec.CommandContext(ctx, "templ", "generate", "-watch")
		setInOuts(templGen)

		err = templGen.Start()
		if err != nil {
			logErrorAndExit(err)
		}

		sig := make(chan os.Signal, 1)
		go signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGABRT, syscall.SIGILL)

		<-sig

		airRun.Wait()
		tailwindWatch.Wait()
		templGen.Wait()

		return nil
	},
}

func setInOuts(cmd *exec.Cmd) {
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
}
