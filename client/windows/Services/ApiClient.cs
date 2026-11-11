using System;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using ScreenTimeClient.Config;
using ScreenTimeClient.Models;

namespace ScreenTimeClient.Services
{
    public class ApiClient
    {
        private readonly HttpClient _http;
        private readonly DeviceConfig _config;

        public ApiClient(DeviceConfig config)
        {
            _config = config;
            _http = new HttpClient
            {
                BaseAddress = new Uri(config.ServerUrl),
                Timeout = TimeSpan.FromSeconds(5)
            };
        }

        private void SetAuthHeader()
        {
            if (!string.IsNullOrEmpty(_config.AuthToken))
            {
                _http.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", _config.AuthToken);
            }
        }

        public async Task<PairDeviceResponse?> PairDeviceAsync(string pairingCode, string deviceName)
        {
            try
            {
                var req = new PairDeviceRequest
                {
                    PairingCode = pairingCode.Trim().ToUpperInvariant(),
                    DeviceName = deviceName,
                    Platform = "WINDOWS"
                };

                var resp = await _http.PostAsJsonAsync("/api/devices/pair", req);
                if (resp.IsSuccessStatusCode)
                {
                    return await resp.Content.ReadFromJsonAsync<PairDeviceResponse>();
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine($"PairDevice error: {ex.Message}");
            }

            return null;
        }

        public async Task<AccessStatusResponse?> GetAccessStatusAsync(CancellationToken ct = default)
        {
            try
            {
                SetAuthHeader();
                var resp = await _http.GetAsync("/api/devices/access-status", ct);
                if (resp.IsSuccessStatusCode)
                {
                    return await resp.Content.ReadFromJsonAsync<AccessStatusResponse>(cancellationToken: ct);
                }
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine($"GetAccessStatus error: {ex.Message}");
            }

            return null;
        }

        public async Task<bool> RequestScreenTimeAsync()
        {
            try
            {
                SetAuthHeader();
                var resp = await _http.PostAsync("/api/requests", null);
                return resp.IsSuccessStatusCode;
            }
            catch (Exception ex)
            {
                System.Diagnostics.Debug.WriteLine($"RequestScreenTime error: {ex.Message}");
                return false;
            }
        }

        public async Task SendHeartbeatAsync(long uptimeSeconds, string status)
        {
            try
            {
                SetAuthHeader();
                var req = new HeartbeatRequest
                {
                    UptimeSeconds = uptimeSeconds,
                    ReportedStatus = status
                };
                await _http.PostAsJsonAsync("/api/devices/heartbeat", req);
            }
            catch { }
        }
    }
}

