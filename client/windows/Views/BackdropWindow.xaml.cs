using System;
using System.Windows;
using System.Windows.Interop;
using ScreenTimeClient.Native;

namespace ScreenTimeClient.Views
{
    public partial class BackdropWindow : Window
    {
        public BackdropWindow(Win32.MonitorInfo monitor)
        {
            InitializeComponent();

            Left = monitor.Bounds.Left;
            Top = monitor.Bounds.Top;
            Width = monitor.Bounds.Width;
            Height = monitor.Bounds.Height;
        }

        protected override void OnSourceInitialized(EventArgs e)
        {
            base.OnSourceInitialized(e);

            // Hook window messages to maintain topmost status
            var hwnd = new WindowInteropHelper(this).Handle;
            var src = HwndSource.FromHwnd(hwnd);
            src?.AddHook(WndProc);
        }

        private IntPtr WndProc(IntPtr hwnd, int msg, IntPtr wParam, IntPtr lParam, ref bool handled)
        {
            const int WM_WINDOWPOSCHANGING = 0x0046;
            if (msg == WM_WINDOWPOSCHANGING)
            {
                // Force window to stay topmost
                Topmost = true;
            }
            return IntPtr.Zero;
        }
    }
}
