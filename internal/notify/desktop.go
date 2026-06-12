package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// execDesktopCommand runs the platform notification command; injectable for
// tests.
var execDesktopCommand = func(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// windowsToastScript shows a native toast via the WinRT projection. It must
// run under Windows PowerShell 5 (powershell.exe) — pwsh 7 does not project
// WinRT types by default.
const windowsToastScript = `[void][Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime];` +
	`$t=[Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02);` +
	`$x=$t.GetElementsByTagName('text');` +
	`[void]$x.Item(0).AppendChild($t.CreateTextNode('%s'));` +
	`[void]$x.Item(1).AppendChild($t.CreateTextNode('%s'));` +
	`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('contrabass').Show([Windows.UI.Notifications.ToastNotification]::new($t))`

// desktopCommand builds the OS-native notification invocation for the given
// platform. ok is false on unsupported platforms.
func desktopCommand(platform, title, message string) (name string, args []string, ok bool) {
	switch platform {
	case "windows":
		script := fmt.Sprintf(windowsToastScript,
			escapePowerShellSingleQuoted(title), escapePowerShellSingleQuoted(message))
		return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", script}, true
	case "darwin":
		script := fmt.Sprintf("display notification %q with title %q", message, title)
		return "osascript", []string{"-e", script}, true
	case "linux":
		return "notify-send", []string{title, message}, true
	default:
		return "", nil, false
	}
}

func escapePowerShellSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// notifyDesktop shows an OS notification; failures are returned for the
// caller to log (a missing notify-send must never affect runs).
func notifyDesktop(title, message string) error {
	name, args, ok := desktopCommand(runtime.GOOS, title, message)
	if !ok {
		return fmt.Errorf("desktop notifications are not supported on %s", runtime.GOOS)
	}
	return execDesktopCommand(name, args...)
}
