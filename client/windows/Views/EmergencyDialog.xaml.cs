using System.Windows;
using ScreenTimeClient.Config;

namespace ScreenTimeClient.Views
{
    public partial class EmergencyDialog : Window
    {
        public EmergencyDialog(DeviceConfig config)
        {
            InitializeComponent();
            if (!string.IsNullOrEmpty(config.EmergencyContact))
            {
                PhoneText.Text = config.EmergencyContact;
            }
        }

        private void OnCloseClicked(object sender, RoutedEventArgs e)
        {
            Close();
        }
    }
}

