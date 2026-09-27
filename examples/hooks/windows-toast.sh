#!/bin/sh
# Transition hook (WSL): a Windows toast notification per transition.
#
# powershell.exe takes ~0.5s just to start, longer than the default hook
# timeout, so the toast is launched fully detached and this hook returns at
# once. That also means it is slow and noisy: a demonstration of crossing
# the WSL boundary, not a way to drive a keyboard layer (use fifo.sh and a
# long-lived listener for that).
msg="${MODAL_APP:-shell}: ${MODAL_MODE_TO} (typing=${MODAL_TYPING})"
# Toasts need a registered AppUserModelID; Windows PowerShell's own works
# without installing anything.
aumid='{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
ps="
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > \$null
\$x = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
\$t = \$x.GetElementsByTagName('text')
\$t.Item(0).AppendChild(\$x.CreateTextNode('tmux-modal')) > \$null
\$t.Item(1).AppendChild(\$x.CreateTextNode(\$env:MODAL_MSG)) > \$null
\$n = [Windows.UI.Notifications.ToastNotification]::new(\$x)
\$n.ExpirationTime = [DateTimeOffset]::Now.AddSeconds(4)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('$aumid').Show(\$n)
"
# WSLENV forwards MODAL_MSG into the Windows process's environment.
MODAL_MSG="$msg" WSLENV="${WSLENV:+$WSLENV:}MODAL_MSG" \
	setsid powershell.exe -NoProfile -NonInteractive -Command "$ps" >/dev/null 2>&1 </dev/null &
