using System;
using System.Text.Json.Serialization;

namespace ScreenTimeClient.Models
{
    public class PairDeviceRequest
    {
        [JsonPropertyName("pairing_code")]
        public string PairingCode { get; set; } = string.Empty;

        [JsonPropertyName("platform")]
        public string Platform { get; set; } = "WINDOWS";

        [JsonPropertyName("device_name")]
        public string DeviceName { get; set; } = string.Empty;
    }

    public class PairDeviceResponse
    {
        [JsonPropertyName("device_id")]
        public string DeviceId { get; set; } = string.Empty;

        [JsonPropertyName("auth_token")]
        public string AuthToken { get; set; } = string.Empty;

        [JsonPropertyName("child_id")]
        public string ChildId { get; set; } = string.Empty;

        [JsonPropertyName("child_display_name")]
        public string ChildDisplayName { get; set; } = string.Empty;

        [JsonPropertyName("emergency_contact")]
        public string EmergencyContact { get; set; } = string.Empty;
    }

    public class AccessStatusResponse
    {
        [JsonPropertyName("access")]
        public string Access { get; set; } = "LOCKED"; // "ALLOWED" | "LOCKED"

        [JsonPropertyName("reason")]
        public string Reason { get; set; } = "NONE"; // "SCHEDULE" | "GRANT" | "NONE"

        [JsonPropertyName("valid_until")]
        public DateTime? ValidUntil { get; set; }

        [JsonPropertyName("remaining_seconds")]
        public long RemainingSeconds { get; set; }

        [JsonPropertyName("emergency_contact")]
        public string EmergencyContact { get; set; } = string.Empty;

        [JsonPropertyName("child_display_name")]
        public string ChildDisplayName { get; set; } = string.Empty;
    }

    public class HeartbeatRequest
    {
        [JsonPropertyName("uptime_seconds")]
        public long UptimeSeconds { get; set; }

        [JsonPropertyName("reported_status")]
        public string ReportedStatus { get; set; } = "LOCKED";
    }
}

