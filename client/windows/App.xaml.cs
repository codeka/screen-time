using System;
using System.Diagnostics;
using System.Threading;
using System.Windows;
using ScreenTimeClient.Config;
using ScreenTimeClient.Native;
using ScreenTimeClient.Services;
using ScreenTimeClient.Views;

namespace ScreenTimeClient
{
    public partial class App : Application
    {
        private const string MutexName = @"Local\CodekaScreenTimeClient";
        private Mutex? _instanceMutex;
        private string? _serverOverride;
        private DeviceConfig _config = null!;
        private ApiClient _api = null!;
        private RealtimeClient _realtime = null!;
        private LockManager? _lockManager;
        private UpdateService _updater = null!;

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
            bool forceCheckUpdates = false;
            bool disableUpdates = false;

            for (int i = 0; i < e.Args.Length; i++)
            {
                if (e.Args[i].StartsWith("--server="))
                {
                    serverOverride = e.Args[i].Substring("--server=".Length).Trim('"');
                }
                else if (e.Args[i] == "--server" && i + 1 < e.Args.Length)
                {
                    serverOverride = e.Args[i + 1].Trim('"');
                }
                else if (e.Args[i] == "--check-updates")
                {
                    forceCheckUpdates = true;
                }
                else if (e.Args[i] == "--no-update")
                {
                    disableUpdates = true;
                }
            }

            _serverOverride = serverOverride;
            _config = DeviceConfig.Load(serverOverride);

            _updater = new UpdateService(
                _config,
                forceEnable: forceCheckUpdates,
                forceDisable: disableUpdates,
                canApplyNow: () => !_config.IsPaired || (_lockManager?.IsLocked ?? true),
                launchInstallerAndExit: ApplyUpdateAndShutdown
            );
            _updater.Start();

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
            _lockManager.OnLocked += () =>
            {
                _updater.ApplyStagedUpdateIfSafe();
            };
            await _lockManager.StartAsync();
        }

        private void ApplyUpdateAndShutdown(string installerPath)
        {
            Dispatcher.Invoke(() =>
            {
                try
                {
                    Win32.UninstallKeyboardHook();
                }
                catch { }

                try { _instanceMutex?.ReleaseMutex(); } catch { }
                try { _instanceMutex?.Dispose(); } catch { }
                _instanceMutex = null;

                var args = "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /CLOSEAPPLICATIONS /RELAUNCH=1";
                if (!string.IsNullOrWhiteSpace(_serverOverride))
                {
                    args += $" /SERVERURL=\"{_serverOverride}\"";
                }

                try
                {
                    Process.Start(new ProcessStartInfo
                    {
                        FileName = installerPath,
                        Arguments = args,
                        UseShellExecute = true
                    });
                }
                catch (Exception ex)
                {
                    Debug.WriteLine($"Failed to launch update installer: {ex.Message}");
                    return;
                }

                Shutdown();
            });
        }

        protected override void OnExit(ExitEventArgs e)
        {
            try { _instanceMutex?.Dispose(); } catch { }
            base.OnExit(e);
        }
    }
}
