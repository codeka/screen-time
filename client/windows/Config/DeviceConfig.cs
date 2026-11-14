using System;
using System.IO;
using System.Text.Json;

namespace ScreenTimeClient.Config
{
    public class DeviceConfig
    {
        public string ServerUrl { get; set; } = "https://screentime.codeka.com";
        public string DeviceId { get; set; } = string.Empty;
        public string AuthToken { get; set; } = string.Empty;
        public string ChildId { get; set; } = string.Empty;
        public string ChildDisplayName { get; set; } = string.Empty;
        public string EmergencyContact { get; set; } = string.Empty;

        public bool IsPaired => !string.IsNullOrEmpty(DeviceId) && !string.IsNullOrEmpty(AuthToken);

        private string _configFilePath = DefaultConfigPath;

        private static string DefaultConfigPath => Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
            "ScreenTime",
            "config.json"
        );

        public static DeviceConfig Load(string? overrideServerUrl = null)
        {
            string targetPath = DefaultConfigPath;
            if (!string.IsNullOrWhiteSpace(overrideServerUrl))
            {
                // Use a separate config file per custom server so production config.json is never overwritten
                var safeHost = overrideServerUrl.Trim()
                    .Replace("https://", "")
                    .Replace("http://", "")
                    .Replace(":", "_")
                    .Replace("/", "_");
                targetPath = Path.Combine(
                    Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                    "ScreenTime",
                    $"config.{safeHost}.json"
                );
            }

            try
            {
                if (File.Exists(targetPath))
                {
                    var json = File.ReadAllText(targetPath);
                    var cfg = JsonSerializer.Deserialize<DeviceConfig>(json);
                    if (cfg != null)
                    {
                        cfg._configFilePath = targetPath;
                        if (!string.IsNullOrWhiteSpace(overrideServerUrl))
                        {
                            cfg.ServerUrl = overrideServerUrl.TrimEnd('/');
                        }
                        return cfg;
                    }
                }
            }
            catch { }

            var fresh = new DeviceConfig
            {
                _configFilePath = targetPath
            };
            if (!string.IsNullOrWhiteSpace(overrideServerUrl))
            {
                fresh.ServerUrl = overrideServerUrl.TrimEnd('/');
            }
            return fresh;
        }

        public void Save()
        {
            try
            {
                var dir = Path.GetDirectoryName(_configFilePath);
                if (!string.IsNullOrEmpty(dir) && !Directory.Exists(dir))
                {
                    Directory.CreateDirectory(dir);
                }

                var json = JsonSerializer.Serialize(this, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(_configFilePath, json);
            }
            catch { }
        }

        public void Clear()
        {
            DeviceId = string.Empty;
            AuthToken = string.Empty;
            ChildId = string.Empty;
            ChildDisplayName = string.Empty;
            EmergencyContact = string.Empty;
            Save();
        }
    }
}

