[Setup]
AppName=Walkie Talkie
AppVersion=1.0
DefaultDirName={pf}\WalkieTalkie
DefaultGroupName=WalkieTalkie
OutputDir=release
OutputBaseFilename=WalkieTalkieInstaller
Compression=lzma
SolidCompression=yes

[Files]
Source: "build/windows/walkietalkie-windows.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "build/windows/portaudio.dll"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Walkie Talkie"; Filename: "{app}\walkietalkie-windows.exe"