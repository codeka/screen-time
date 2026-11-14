using System;
using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Net.Http.Json;
using System.Reflection;
using System.Security.Cryptography;
using System.Text.Json.Serialization;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Threading;
using Microsoft.Win32;
using ScreenTimeClient.Config;

namespace ScreenTimeClient.Services
{
    public class WindowsUpdateManifest
    {
        [JsonPropertyName("version")]
        public string Version { get; set; } = string.Empty;

        [JsonPropertyName("sha256")]
        public string Sha256 { get; set; } = string.Empty;

        [JsonPropertyName("url")]
        public string Url { get; set; } = "/downloads/ScreenTimeSetup.exe";

        [JsonPropertyName("size_bytes")]
        public long SizeBytes { get; set; }
    }

    public class UpdateService
    {
        private readonly DeviceConfig _config;
        private readonly HttpClient _http;
        private readonly Func<bool> _canApplyNow;
        private readonly Action<string> _launchInstallerAndExit;
        private readonly DispatcherTimer _periodicTimer;
        private readonly SemaphoreSlim _checkGate = new(1, 1);
        private readonly bool _enabled;

        private string? _stagedInstallerPath;
        private DateTime _lastCheckUtc = DateTime.MinValue;

        public UpdateService(
            DeviceConfig config,
            bool forceEnable,
            bool forceDisable,
            Func<bool> canApplyNow,
            Action<string> launchInstallerAndExit)
        {
            _config = config;
            _canApplyNow = canApplyNow;
            _launchInstallerAndExit = launchInstallerAndExit;
            _enabled = DetermineIfEnabled(forceEnable, forceDisable);

            _http = new HttpClient
            {
                BaseAddress = new Uri(_config.ServerUrl.TrimEnd('/') + "/"),
                Timeout = TimeSpan.FromMinutes(5)
            };

            // Re-check every 4 hours for machines that sleep/wake instead of rebooting
            _periodicTimer = new DispatcherTimer
            {
                Interval = TimeSpan.FromHours(4)
            };
            _periodicTimer.Tick += async (s, e) => await CheckForUpdatesAsync(applyImmediately: false);
        }

        private static bool DetermineIfEnabled(bool forceEnable, bool forceDisable)
        {
            if (forceDisable) return false;
            if (forceEnable) return true;

            var baseDir = AppContext.BaseDirectory.Replace('/', '\\').ToLowerInvariant();
            if (baseDir.Contains(@"\bin\debug\") ||
                baseDir.Contains(@"\bin\release\") ||
                baseDir.Contains(@"\publish\") ||
                baseDir.Contains(@"\publish_staging\"))
            {
                return false;
            }

            return true;
        }

        public static Version GetCurrentVersion()
        {
            return Assembly.GetEntryAssembly()?.GetName().Version ?? new Version(1, 0, 0, 0);
        }

        private static Version NormalizeVersion(Version v)
        {
            return new Version(
                v.Major,
                v.Minor,
                Math.Max(0, v.Build),
                Math.Max(0, v.Revision)
            );
        }

        public void Start()
        {
            if (!_enabled) return;

            _periodicTimer.Start();
            SystemEvents.SessionSwitch += OnSessionSwitch;

            // Fire non-blocking startup update check
            _ = CheckForUpdatesAsync(applyImmediately: true);
        }

        private async void OnSessionSwitch(object sender, SessionSwitchEventArgs e)
        {
            if (!_enabled) return;

            if (e.Reason == SessionSwitchReason.SessionUnlock || e.Reason == SessionSwitchReason.SessionLogon)
            {
                if (ApplyStagedUpdateIfSafe()) return;

                if ((DateTime.UtcNow - _lastCheckUtc) > TimeSpan.FromMinutes(30))
                {
                    await CheckForUpdatesAsync(applyImmediately: false);
                }
            }
        }

        public bool ApplyStagedUpdateIfSafe()
        {
            if (!_enabled) return false;
            if (!string.IsNullOrEmpty(_stagedInstallerPath) && File.Exists(_stagedInstallerPath) && _canApplyNow())
            {
                var path = _stagedInstallerPath;
                _stagedInstallerPath = null;
                _launchInstallerAndExit(path);
                return true;
            }
            return false;
        }

        public async Task CheckForUpdatesAsync(bool applyImmediately)
        {
            if (!_enabled) return;
            if (!await _checkGate.WaitAsync(0)) return;

            try
            {
                _lastCheckUtc = DateTime.UtcNow;

                var manifest = await _http.GetFromJsonAsync<WindowsUpdateManifest>("api/updates/windows");
                if (manifest == null ||
                    string.IsNullOrWhiteSpace(manifest.Version) ||
                    string.IsNullOrWhiteSpace(manifest.Sha256))
                {
                    return;
                }

                if (!Version.TryParse(manifest.Version.Trim(), out var remoteVersion))
                {
                    return;
                }

                var currentNorm = NormalizeVersion(GetCurrentVersion());
                var remoteNorm = NormalizeVersion(remoteVersion);
                if (remoteNorm <= currentNorm)
                {
                    return;
                }

                var updateDir = Path.Combine(Path.GetTempPath(), "ScreenTimeUpdates");
                Directory.CreateDirectory(updateDir);

                var finalInstallerPath = Path.Combine(updateDir, $"ScreenTimeSetup-{manifest.Version.Trim()}.exe");

                // Reuse already-staged installer if its SHA-256 matches
                bool needsDownload = true;
                if (File.Exists(finalInstallerPath))
                {
                    var existingHash = await ComputeFileSha256Async(finalInstallerPath);
                    if (string.Equals(existingHash, manifest.Sha256.Trim(), StringComparison.OrdinalIgnoreCase))
                    {
                        needsDownload = false;
                    }
                }

                if (needsDownload)
                {
                    CleanupOldUpdateFiles(updateDir, finalInstallerPath);

                    var tempPartPath = finalInstallerPath + ".part";
                    var downloadUrl = manifest.Url.StartsWith("http", StringComparison.OrdinalIgnoreCase)
                        ? manifest.Url
                        : manifest.Url.TrimStart('/');

                    using (var resp = await _http.GetAsync(downloadUrl, HttpCompletionOption.ResponseHeadersRead))
                    {
                        resp.EnsureSuccessStatusCode();
                        await using var netStream = await resp.Content.ReadAsStreamAsync();
                        await using var fileStream = new FileStream(tempPartPath, FileMode.Create, FileAccess.Write, FileShare.None, 81920, useAsync: true);
                        await netStream.CopyToAsync(fileStream);
                    }

                    var actualHash = await ComputeFileSha256Async(tempPartPath);
                    if (!string.Equals(actualHash, manifest.Sha256.Trim(), StringComparison.OrdinalIgnoreCase))
                    {
                        Debug.WriteLine($"[UpdateService] SHA-256 mismatch: expected {manifest.Sha256}, got {actualHash}");
                        try { File.Delete(tempPartPath); } catch { }
                        return;
                    }

                    File.Move(tempPartPath, finalInstallerPath, overwrite: true);
                }

                _stagedInstallerPath = finalInstallerPath;

                if (applyImmediately || _canApplyNow())
                {
                    _stagedInstallerPath = null;
                    _launchInstallerAndExit(finalInstallerPath);
                }
            }
            catch (Exception ex)
            {
                Debug.WriteLine($"[UpdateService] Update check failed: {ex.Message}");
            }
            finally
            {
                _checkGate.Release();
            }
        }

        private static async Task<string> ComputeFileSha256Async(string filePath)
        {
            await using var stream = new FileStream(filePath, FileMode.Open, FileAccess.Read, FileShare.Read, 81920, useAsync: true);
            var hashBytes = await SHA256.HashDataAsync(stream);
            return Convert.ToHexString(hashBytes).ToLowerInvariant();
        }

        private static void CleanupOldUpdateFiles(string dir, string keepPath)
        {
            try
            {
                foreach (var file in Directory.GetFiles(dir))
                {
                    if (!string.Equals(file, keepPath, StringComparison.OrdinalIgnoreCase))
                    {
                        try { File.Delete(file); } catch { }
                    }
                }
            }
            catch { }
        }
    }
}
