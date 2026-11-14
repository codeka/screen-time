using System.Threading;
using System.Windows;
using ScreenTimeClient.Config;
using ScreenTimeClient.Services;
using ScreenTimeClient.Views;

namespace ScreenTimeClient
{
    public partial class App : Application
    {
        private const string MutexName = @"Local\CodekaScreenTimeClient";
        private Mutex? _instanceMutex;
        private DeviceConfig _config = null!;
        private ApiClient _api = null!;
        private RealtimeClient _realtime = null!;
        private LockManager _lockManager = null!;

        protected override async void OnStartup(StartupEventArgs e)
        {
            base.OnStartup(e);

            _instanceMutex = new Mutex(true, MutexName, out bool createdNew);
            if (!createdNew)
            {
                // Another instance is already running in this user session
                Shutdown();
                return;
            }

            string? serverOverride = Environment.GetEnvironmentVariable("SCREENTIME_SERVER");
            for (int i = 0; i < e.Args.Length; i++)
            {
                if (e.Args[i].StartsWith("--server="))
                {
                    serverOverride = e.Args[i].Substring("--server=".Length);
                }
                else if (e.Args[i] == "--server" && i + 1 < e.Args.Length)
                {
                    serverOverride = e.Args[i + 1];
                }
            }

            _config = DeviceConfig.Load(serverOverride);

            if (!_config.IsPaired)
            {
                ShowPairingWindow();
            }
            else
            {
                await StartEnforcementAsync();
            }
        }

        private void ShowPairingWindow()
        {
            _api = new ApiClient(_config);
            var pairingWin = new PairingWindow(_config, _api);
            pairingWin.OnPairingSuccessful += async () =>
            {
                await StartEnforcementAsync();
            };
            pairingWin.Closed += (s, args) =>
            {
                if (!_config.IsPaired)
                {
                    Shutdown();
                }
            };
            pairingWin.Show();
        }

        private async System.Threading.Tasks.Task StartEnforcementAsync()
        {
            _api = new ApiClient(_config);
            _realtime = new RealtimeClient(_config);
            _lockManager = new LockManager(_config, _api, _realtime);
            _lockManager.OnDeregistered += () =>
            {
                ShowPairingWindow();
            };
            await _lockManager.StartAsync();
        }

        protected override void OnExit(ExitEventArgs e)
        {
            _instanceMutex?.Dispose();
            base.OnExit(e);
        }
    }
}
