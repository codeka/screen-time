using System;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using ScreenTimeClient.Config;

namespace ScreenTimeClient.Services
{
    public class RealtimeClient
    {
        private readonly DeviceConfig _config;
        private ClientWebSocket? _ws;
        private CancellationTokenSource? _cts;

        public event Action<string, JsonElement>? OnEventReceived;

        public RealtimeClient(DeviceConfig config)
        {
            _config = config;
        }

        public void Start()
        {
            if (!_config.IsPaired) return;

            _cts?.Cancel();
            _cts = new CancellationTokenSource();
            _ = RunLoopAsync(_cts.Token);
        }

        public void Stop()
        {
            _cts?.Cancel();
            try { _ws?.Dispose(); } catch { }
            _ws = null;
        }

        private async Task RunLoopAsync(CancellationToken ct)
        {
            while (!ct.IsCancellationRequested)
            {
                try
                {
                    var baseUri = new Uri(_config.ServerUrl);
                    var scheme = baseUri.Scheme == "https" ? "wss" : "ws";
                    var wsUri = new Uri($"{scheme}://{baseUri.Authority}/api/ws/device?token={Uri.EscapeDataString(_config.AuthToken)}");

                    _ws = new ClientWebSocket();
                    await _ws.ConnectAsync(wsUri, ct);

                    var buffer = new byte[4096];
                    while (_ws.State == WebSocketState.Open && !ct.IsCancellationRequested)
                    {
                        var result = await _ws.ReceiveAsync(new ArraySegment<byte>(buffer), ct);
                        if (result.MessageType == WebSocketMessageType.Close)
                        {
                            break;
                        }

                        if (result.MessageType == WebSocketMessageType.Text)
                        {
                            var text = Encoding.UTF8.GetString(buffer, 0, result.Count);
                            using var doc = JsonDocument.Parse(text);
                            if (doc.RootElement.TryGetProperty("type", out var typeProp))
                            {
                                var type = typeProp.GetString() ?? string.Empty;
                                OnEventReceived?.Invoke(type, doc.RootElement.Clone());
                            }
                        }
                    }
                }
                catch (Exception)
                {
                    // Fallback reconnect delay
                }

                try
                {
                    await Task.Delay(4000, ct);
                }
                catch { }
            }
        }
    }
}

