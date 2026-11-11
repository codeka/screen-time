using System;
using System.Windows;
using System.Windows.Interop;
using System.Windows.Threading;
using ScreenTimeClient.Config;
using ScreenTimeClient.Native;
using ScreenTimeClient.Services;

namespace ScreenTimeClient.Views
{
    public partial class LockOverlayWindow : Window
    {
        private readonly DeviceConfig _config;
        private readonly ApiClient _api;
        private readonly DispatcherTimer _clockTimer;

        public LockOverlayWindow(Win32.MonitorInfo monitor, DeviceConfig config, ApiClient api)
        {
            InitializeComponent();
            _config = config;
            _api = api;

            Left = monitor.Bounds.Left;
            Top = monitor.Bounds.Top;
            Width = monitor.Bounds.Width;
            Height = monitor.Bounds.Height;

            ChildGreetingText.Text = string.IsNullOrEmpty(config.ChildDisplayName)
                ? "Welcome"
                : $"Hi, {config.ChildDisplayName}!";

            UpdateClock();
            _clockTimer = new DispatcherTimer
            {
                Interval = TimeSpan.FromSeconds(1)
            };
            _clockTimer.Tick += (s, e) => UpdateClock();
            _clockTimer.Start();
        }

        private void UpdateClock()
        {
            var now = DateTime.Now;
            ClockText.Text = now.ToString("h:mm tt");
            DateText.Text = now.ToString("dddd, MMMM d");
        }

        private async void OnRequestClicked(object sender, RoutedEventArgs e)
        {
            RequestButton.IsEnabled = false;
            RequestButtonText.Text = "Sending Request...";

            bool success = await _api.RequestScreenTimeAsync();
            if (success)
            {
                RequestButtonText.Text = "Waiting for Parent...";
                StatusSubtext.Text = "Request sent to your parent's phone! When they approve, this screen will unlock.";
            }
            else
            {
                RequestButtonText.Text = "Try Again";
                RequestButton.IsEnabled = true;
                StatusSubtext.Text = "Could not send request. Please check Wi-Fi connection.";
            }
        }

        private void OnEmergencyClicked(object sender, RoutedEventArgs e)
        {
            var dlg = new EmergencyDialog(_config)
            {
                Owner = this
            };
            dlg.ShowDialog();
        }

        protected override void OnSourceInitialized(EventArgs e)
        {
            base.OnSourceInitialized(e);
            var hwnd = new WindowInteropHelper(this).Handle;
            var src = HwndSource.FromHwnd(hwnd);
            src?.AddHook(WndProc);
        }

        private IntPtr WndProc(IntPtr hwnd, int msg, IntPtr wParam, IntPtr lParam, ref bool handled)
        {
            const int WM_WINDOWPOSCHANGING = 0x0046;
            if (msg == WM_WINDOWPOSCHANGING)
            {
                Topmost = true;
            }
            return IntPtr.Zero;
        }

        protected override void OnClosed(EventArgs e)
        {
            _clockTimer.Stop();
            base.OnClosed(e);
        }
    }
}
