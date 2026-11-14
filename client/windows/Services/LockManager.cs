using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Linq;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Threading;
using Microsoft.Win32;
using ScreenTimeClient.Config;
using ScreenTimeClient.Native;
using ScreenTimeClient.Views;

namespace ScreenTimeClient.Services
{
    public class LockManager
    {
        private readonly DeviceConfig _config;
        private readonly ApiClient _api;
        private readonly RealtimeClient _realtime;

        private readonly List<Window> _activeOverlayWindows = new();
        private Window? _activeToastWindow;
        private readonly DispatcherTimer _countdownTimer;
        private readonly DispatcherTimer _heartbeatTimer;

        public bool IsLocked { get; private set; } = true;
        private long _remainingSeconds = 0;
        private bool _warned5Min = false;
        private bool _warned1Min = false;

        public LockManager(DeviceConfig config, ApiClient api, RealtimeClient realtime)
        {
            _config = config;
            _api = api;
            _realtime = realtime;

            _countdownTimer = new DispatcherTimer { Interval = TimeSpan.FromSeconds(1) };
            _countdownTimer.Tick += OnCountdownTick;

            _heartbeatTimer = new DispatcherTimer { Interval = TimeSpan.FromSeconds(60) };
            _heartbeatTimer.Tick += (s, e) => SendHeartbeat();

            // Hook workstation unlock & display changes
            SystemEvents.SessionSwitch += OnSessionSwitch;
            SystemEvents.DisplaySettingsChanged += OnDisplaySettingsChanged;

            // Hook realtime events
            _realtime.OnEventReceived += OnRealtimeEvent;

            // Developer Safety Escape: Ctrl + Alt + Shift + Q instantly unlocks for 2 hours!
            Win32.OnEmergencyEscape += () =>
            {
                Unlock(DateTime.Now.AddHours(2), 7200);
            };
        }

        public async Task StartAsync()
        {
            _realtime.Start();
            _heartbeatTimer.Start();
            SendHeartbeat();

            // Run pre-popup check on startup
            await CheckAccessAndApplyAsync();
        }

        private async void OnSessionSwitch(object sender, SessionSwitchEventArgs e)
        {
            if (e.Reason == SessionSwitchReason.SessionUnlock || e.Reason == SessionSwitchReason.SessionLogon)
            {
                // Run fast pre-popup check
                await CheckAccessAndApplyAsync();
            }
        }

        public async Task CheckAccessAndApplyAsync()
        {
            var status = await _api.GetAccessStatusAsync();
            if (status != null)
            {
                if (status.Access == "ALLOWED")
                {
                    Unlock(status.ValidUntil, status.RemainingSeconds);
                    return;
                }
            }

            // Otherwise, lock
            Lock();
        }

        public void Lock()
        {
            Application.Current.Dispatcher.Invoke(() =>
            {
                if (IsLocked && _activeOverlayWindows.Count > 0)
                {
                    return; // Already locked
                }

                IsLocked = true;
                _countdownTimer.Stop();

                // Close any stale overlay or warning toast windows
                CloseToast();
                CloseOverlays();

                // Multi-monitor deployment
                var monitors = Win32.GetAllMonitors();
                foreach (var mon in monitors)
                {
                    if (mon.IsPrimary)
                    {
                        var primaryWin = new LockOverlayWindow(mon, _config, _api);
                        _activeOverlayWindows.Add(primaryWin);
                        primaryWin.Show();
                    }
                    else
                    {
                        var secWin = new BackdropWindow(mon);
                        _activeOverlayWindows.Add(secWin);
                        secWin.Show();
                    }
                }

                // Install low-level keyboard hook
                Win32.InstallKeyboardHook();
            });
        }

        public void Unlock(DateTime? validUntil, long remainingSeconds)
        {
            Application.Current.Dispatcher.Invoke(() =>
            {
                IsLocked = false;
                _remainingSeconds = remainingSeconds > 0 ? remainingSeconds : 1800;
                _warned5Min = false;
                _warned1Min = false;

                // Dismiss all overlays & active warning toast
                CloseToast();
                CloseOverlays();

                // Uninstall keyboard hook
                Win32.UninstallKeyboardHook();

                // Start local countdown
                _countdownTimer.Start();
            });
        }

        private void CloseOverlays()
        {
            foreach (var win in _activeOverlayWindows.ToList())
            {
                try { win.Close(); } catch { }
            }
            _activeOverlayWindows.Clear();
        }

        private void OnCountdownTick(object? sender, EventArgs e)
        {
            if (IsLocked) return;

            _remainingSeconds--;

            // 5 minute warning
            if (_remainingSeconds <= 300 && _remainingSeconds > 290 && !_warned5Min)
            {
                _warned5Min = true;
                ShowNotificationToast("Screen Time: 5 Minutes Remaining", "Your screen time will expire in 5 minutes.");
            }

            // 1 minute warning
            if (_remainingSeconds <= 60 && _remainingSeconds > 50 && !_warned1Min)
            {
                _warned1Min = true;
                ShowNotificationToast("Screen Time: 1 Minute Remaining", "Please finish up what you are doing. The screen will lock shortly.");
            }

            // Time expired
            if (_remainingSeconds <= 0)
            {
                Lock();
            }
        }

        private void CloseToast()
        {
            if (_activeToastWindow != null)
            {
                try { _activeToastWindow.Close(); } catch { }
                _activeToastWindow = null;
            }
        }

        private void ShowNotificationToast(string title, string message)
        {
            Application.Current.Dispatcher.Invoke(() =>
            {
                CloseToast();

                // Unobtrusive popup notification window in bottom-right corner of screen
                var toast = new Window
                {
                    Title = title,
                    Width = 350,
                    SizeToContent = SizeToContent.Height,
                    WindowStyle = WindowStyle.None,
                    ResizeMode = ResizeMode.NoResize,
                    ShowInTaskbar = false,
                    ShowActivated = false,
                    Topmost = true,
                    Background = System.Windows.Media.Brushes.Transparent,
                    AllowsTransparency = true
                };
                _activeToastWindow = toast;

                var toastTimer = new DispatcherTimer { Interval = TimeSpan.FromSeconds(15) };
                toastTimer.Tick += (s, e) =>
                {
                    toastTimer.Stop();
                    if (_activeToastWindow == toast) _activeToastWindow = null;
                    try { toast.Close(); } catch { }
                };

                var border = new System.Windows.Controls.Border
                {
                    Background = new System.Windows.Media.SolidColorBrush(System.Windows.Media.Color.FromRgb(30, 41, 59)),
                    BorderBrush = new System.Windows.Media.SolidColorBrush(System.Windows.Media.Color.FromRgb(245, 158, 11)),
                    BorderThickness = new Thickness(1),
                    CornerRadius = new CornerRadius(10),
                    Padding = new Thickness(14)
                };

                var sp = new System.Windows.Controls.StackPanel();

                var headerRow = new System.Windows.Controls.DockPanel { LastChildFill = true };
                var dismissBtn = new System.Windows.Controls.Button
                {
                    Content = "✕",
                    Background = System.Windows.Media.Brushes.Transparent,
                    BorderThickness = new Thickness(0),
                    Foreground = new System.Windows.Media.SolidColorBrush(System.Windows.Media.Color.FromRgb(148, 163, 184)),
                    FontSize = 12,
                    Padding = new Thickness(4, 0, 0, 0),
                    Cursor = System.Windows.Input.Cursors.Hand
                };
                System.Windows.Controls.DockPanel.SetDock(dismissBtn, System.Windows.Controls.Dock.Right);
                dismissBtn.Click += (s, e) =>
                {
                    toastTimer.Stop();
                    if (_activeToastWindow == toast) _activeToastWindow = null;
                    try { toast.Close(); } catch { }
                };

                var tTitle = new System.Windows.Controls.TextBlock
                {
                    Text = title,
                    FontWeight = FontWeights.Bold,
                    Foreground = System.Windows.Media.Brushes.White,
                    FontSize = 13,
                    TextTrimming = TextTrimming.CharacterEllipsis
                };
                headerRow.Children.Add(dismissBtn);
                headerRow.Children.Add(tTitle);

                var tMsg = new System.Windows.Controls.TextBlock
                {
                    Text = message,
                    Foreground = new System.Windows.Media.SolidColorBrush(System.Windows.Media.Color.FromRgb(203, 213, 225)),
                    FontSize = 11.5,
                    TextWrapping = TextWrapping.Wrap,
                    Margin = new Thickness(0, 4, 0, 10)
                };

                var requestBtn = new System.Windows.Controls.Button
                {
                    Content = "✋ Request More Time",
                    Background = new System.Windows.Media.SolidColorBrush(System.Windows.Media.Color.FromRgb(79, 70, 229)),
                    Foreground = System.Windows.Media.Brushes.White,
                    BorderThickness = new Thickness(0),
                    Padding = new Thickness(12, 7, 12, 7),
                    FontWeight = FontWeights.SemiBold,
                    FontSize = 12,
                    Cursor = System.Windows.Input.Cursors.Hand,
                    HorizontalAlignment = HorizontalAlignment.Stretch
                };

                requestBtn.Click += async (s, e) =>
                {
                    toastTimer.Stop();
                    requestBtn.IsEnabled = false;
                    requestBtn.Content = "Sending Request...";
                    bool ok = await _api.RequestScreenTimeAsync();
                    if (ok)
                    {
                        requestBtn.Content = "✓ Request Sent to Parent!";
                        requestBtn.Background = new System.Windows.Media.SolidColorBrush(System.Windows.Media.Color.FromRgb(22, 163, 74));
                        toastTimer.Interval = TimeSpan.FromSeconds(3);
                        toastTimer.Start();
                    }
                    else
                    {
                        requestBtn.IsEnabled = true;
                        requestBtn.Content = "Retry Request More Time";
                        toastTimer.Interval = TimeSpan.FromSeconds(10);
                        toastTimer.Start();
                    }
                };

                sp.Children.Add(headerRow);
                sp.Children.Add(tMsg);
                sp.Children.Add(requestBtn);
                border.Child = sp;
                toast.Content = border;

                toast.Loaded += (s, e) =>
                {
                    var workArea = SystemParameters.WorkArea;
                    toast.Left = workArea.Right - toast.ActualWidth - 16;
                    toast.Top = workArea.Bottom - toast.ActualHeight - 16;
                };

                toast.Show();
                toastTimer.Start();
            });
        }

        private void OnDisplaySettingsChanged(object? sender, EventArgs e)
        {
            // If locked when an HDMI cable is plugged or resolution changes, recreate overlays across all screens
            if (IsLocked)
            {
                Lock();
            }
        }

        private async void OnRealtimeEvent(string type, System.Text.Json.JsonElement element)
        {
            if (type == "ACCESS_UPDATE")
            {
                if (element.TryGetProperty("access", out var acc) && acc.GetString() == "LOCKED")
                {
                    Lock();
                    return;
                }
                DateTime? validUntil = null;
                if (element.TryGetProperty("valid_until", out var vu))
                {
                    if (DateTime.TryParse(vu.GetString(), out var dt)) validUntil = dt;
                }
                long remaining = 1800;
                if (element.TryGetProperty("remaining_seconds", out var rs))
                {
                    remaining = rs.GetInt64();
                }

                Unlock(validUntil, remaining);
            }
            else if (type == "SCHEDULE_UPDATED")
            {
                await CheckAccessAndApplyAsync();
            }
            else if (type == "REQUEST_DENIED")
            {
                Lock();
            }
        }

        private void SendHeartbeat()
        {
            long uptime = Environment.TickCount64 / 1000;
            _ = _api.SendHeartbeatAsync(uptime, IsLocked ? "LOCKED" : "ACTIVE");
        }
    }
}

