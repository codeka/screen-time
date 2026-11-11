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

        private static string ConfigPath => Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
            "ScreenTime",
            "config.json"
        );

        public static DeviceConfig Load()
        {
            try
            {
                if (File.Exists(ConfigPath))
                {
                    var json = File.ReadAllText(ConfigPath);
                    var cfg = JsonSerializer.Deserialize<DeviceConfig>(json);
                    if (cfg != null) return cfg;
                }
            }
            catch { }

            return new DeviceConfig();
        }

        public void Save()
        {
            try
            {
                var dir = Path.GetDirectoryName(ConfigPath);
                if (!string.IsNullOrEmpty(dir) && !Directory.Exists(dir))
                {
                    Directory.CreateDirectory(dir);
                }

                var json = JsonSerializer.Serialize(this, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(ConfigPath, json);
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

