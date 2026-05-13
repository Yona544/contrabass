package agent

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

func interruptOrKillProcess(process *os.Process) error {
	if process == nil {
		return nil
	}

	if err := process.Signal(os.Interrupt); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		if runtime.GOOS != "windows" {
			return fmt.Errorf("interrupt process: %w", err)
		}
		if killErr := process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return fmt.Errorf("kill process after interrupt failed: %w", killErr)
		}
	}

	return nil
}
