using System;
using System.Windows;
using ScreenTimeClient.Config;
using ScreenTimeClient.Services;

namespace ScreenTimeClient.Views
{
    public partial class PairingWindow : Window
    {
        private readonly DeviceConfig _config;
        private readonly ApiClient _api;

        public event Action? OnPairingSuccessful;

        public PairingWindow(DeviceConfig config, ApiClient api)
        {
            InitializeComponent();
            _config = config;
            _api = api;
            DeviceNameInput.Text = $"{Environment.MachineName}";
            CodeInput.Focus();
        }

        private async void OnPairClicked(object sender, RoutedEventArgs e)
        {
            var code = CodeInput.Text.Trim();
            var devName = DeviceNameInput.Text.Trim();

            if (string.IsNullOrEmpty(code) || code.Length < 4)
            {
                ShowError("Please enter the 6-character pairing code.");
                return;
            }

            PairButton.IsEnabled = false;
            PairButton.Content = "Pairing...";
            ErrorBanner.Visibility = Visibility.Collapsed;

            var res = await _api.PairDeviceAsync(code, string.IsNullOrEmpty(devName) ? Environment.MachineName : devName);
            if (res != null && !string.IsNullOrEmpty(res.AuthToken))
            {
                _config.DeviceId = res.DeviceId;
                _config.AuthToken = res.AuthToken;
                _config.ChildId = res.ChildId;
                _config.ChildDisplayName = res.ChildDisplayName;
                _config.EmergencyContact = res.EmergencyContact;
                _config.Save();

                OnPairingSuccessful?.Invoke();
                Close();
            }
            else
            {
                ShowError("Invalid or expired pairing code. Please generate a fresh code on the web portal.");
                PairButton.IsEnabled = true;
                PairButton.Content = "Pair This PC";
            }
        }

        private void ShowError(string message)
        {
            ErrorText.Text = message;
            ErrorBanner.Visibility = Visibility.Visible;
        }
    }
}
