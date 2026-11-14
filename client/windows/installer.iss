; Inno Setup Script for Screen Time Client
; Builds a traditional Windows .exe installer

#define MyAppName "Screen Time"
#define MyAppVersion "1.0.0"
#define MyAppPublisher "Codeka"
#define MyAppURL "https://screentime.codeka.com"
#define MyAppExeName "ScreenTimeClient.exe"

[Setup]
; NOTE: Double '{{' escapes the literal '{' for the GUID in Inno Setup
AppId={{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
; 64-bit native install
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
; Default to current user (installs to target child's profile & startup folder without UAC profile mismatch,
; and supports future silent auto-updates), while still allowing machine-wide install if chosen
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
OutputDir=installer_output
OutputBaseFilename=ScreenTimeSetup
SetupIconFile=app.ico
UninstallDisplayIcon={app}\{#MyAppExeName}
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
AppMutex=Local\CodekaScreenTimeClient
CloseApplications=force
RestartApplications=no
MinVersion=10.0

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "startupicon"; Description: "Start Screen Time automatically when Windows starts"; GroupDescription: "Startup:"

[Files]
#ifndef PublishDir
  #define PublishDir "publish"
#endif
; Include ALL files from the publish directory (self-contained .NET app)
Source: "{#PublishDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon
Name: "{autostartup}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: startupicon

[Run]
; Launch after install
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Clean up the local config on uninstall
Type: filesandordirs; Name: "{localappdata}\ScreenTime"
