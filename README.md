# gotohp

gotohp is an unofficial uploader for Google Photos. Review files in the desktop app or upload them from a terminal with the standalone CLI.

## Latest downloads

| Platform | Desktop app | Command-line tool (CLI) |
| --- | --- | --- |
| Windows (x64) | [Portable app (.exe)](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp-x64.exe) | [CLI executable (.exe)](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp-cli-x64.exe) |
| macOS (Apple silicon and Intel) | [Disk image (.dmg)](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp-macos-universal.dmg) or [app bundle (.zip)](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp-macos-universal.zip) | [Universal CLI binary](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp-cli-macos-universal) |
| Linux (x86-64) | [AppImage](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp_amd64.AppImage) or [Debian package (.deb)](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp_amd64.deb) | [CLI binary](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp-cli_amd64) or [CLI Debian package (.deb)](https://github.com/Mxlted/gotohp/releases/latest/download/gotohp-cli_amd64.deb) |

[Release notes](https://github.com/Mxlted/gotohp/releases/latest) · [SHA-256 checksums](https://github.com/Mxlted/gotohp/releases/latest/download/SHA256SUMS.txt)

## Desktop uploads

Drop files or folders into the window, or use **Choose files** and **Choose folders**. Review the queue and remove anything you want to leave out. Under **Destination**, select a Google account and choose **Photo library**, **Named album**, or **Albums from folders**. Click **Start upload** to scan the selected folders using your current settings and begin the upload.

![Upload queue with three selected paths and the destination account](readme_assets/upload-review.png)

During an upload, the dashboard shows file progress, transfer speed, and elapsed time. When the batch finishes, **Show details** lists any failures, skips, or warnings. Use **Copy as JSON** to copy the full report.

The upload panel features Ansel Adams’s 1942 photograph of the Tetons and the Snake River. [National Archives photograph](https://www.archives.gov/research/still-pictures/highlights/the-tetons-snake-river)

## Features

- Unlimited uploads (can be disabled)
- Upload individual files or scan folders and their subfolders.
- Skip files already present in the selected account.
- Adjust the number of concurrent uploads.
- Manage multiple Google accounts.

Settings are saved in `gotohp/gotohp.config` under your system configuration directory. To keep settings with the executable, create an empty `gotohp.config` next to it.

## CLI usage

The examples use `gotohp-cli`. If you downloaded the standalone binary, use its filename, such as `.\gotohp-cli-x64.exe` on Windows.

Use `--dry-run` to preview a folder before signing in.

```shell
gotohp-cli upload /path/to/photos --recursive --dry-run
```

The JSON output lists each planned item and its paths. It includes Live Photo pairs, skipped groups, and metadata warnings when pairing is enabled. A dry run reads local media only. Checks for items already in Google Photos take place during an actual upload.

```shell
gotohp-cli upload /path/to/photos --recursive --threads 5
gotohp-cli creds add "<oauth_token cookie value>"   # Embedded Setup sign-in, see below
gotohp-cli creds add "androidId=..."                 # or a raw credential string
gotohp-cli creds add - < token.txt                   # or read either from stdin
gotohp-cli creds set user@gmail.com
```

The CLI reads the saved account from `gotohp.config` and takes upload options from command-line flags. Use `--config <path>` to specify a different config file or `--account <email>` to select a saved account.

Run `gotohp-cli help` or `gotohp-cli upload --help` for all commands and options.

You can also run `gotohp upload /path/to/photos` with the desktop executable.

## Sign in

### Option 1 - Embedded setup

1. Open gotohp and click **Add Google account**.
2. Click **Open Google sign-in** and sign in with the account you want to add.
3. Sign in on the Google Embedded Setup page and click **I agree**. The page may keep loading forever; this is expected.
4. Open the browser developer tools. Under Application or Storage, open Cookies for `accounts.google.com`.
5. Copy only the value of the `oauth_token` cookie and paste it into gotohp.
6. Click **Connect account**.

From the CLI, open the same page yourself and run `gotohp-cli creds add "<oauth_token value>"`.

### Option 2 - ReVanced. No root required

<details>
  <summary><strong>Click to expand</strong></summary>

1. Install Google Photos ReVanced on your android device/emulator.
   - Install GmsCore [https://github.com/ReVanced/GmsCore/releases](https://github.com/ReVanced/GmsCore/releases)
   - Install patched apk [https://github.com/RookieEnough/Morphe-AutoBuilds/releases](https://github.com/RookieEnough/Morphe-AutoBuilds/releases) or patch it yourself
2. Connect the device to your PC via ADB.
3. Open the terminal on your PC and execute

   Windows

   ```cmd
   adb logcat | FINDSTR "auth%2Fphotos.native"
   ```

   Linux/Mac

   ```shell
   adb logcat | grep "auth%2Fphotos.native"
   ```

4. If you are already using ReVanced - remove Google Account from GmsCore.
5. Open Google Photos ReVanced on your device and log into your account.
6. One or more identical GmsCore logs should appear in the terminal.
7. Copy text from `androidId=` to the end of the line from any log.

</details>

### Option 3 - Official apk. Root required

<details>
  <summary><strong>Click to expand</strong></summary>

1. Get a rooted android device or an emulator.
2. Connect the device to your PC via ADB.
3. Install [HTTP Toolkit](https://httptoolkit.com)
4. In HTTP Toolkit, select Intercept - `Android Device via ADB`. Filter traffic with

   ```text
   contains(https://www.googleapis.com/auth/photos.native)
   ```

   Or if you have an older version of Google Photos, try

   ```text
   contains(www.googleapis.com%2Fauth%2Fplus.photos.readwrite)
   ```

5. Open Google Photos app and login with your account.
6. A single request should appear.  
   Copy request body as text.
7. Add that credential string in gotohp.
8. If gotohp asks for a token binding key, keep the rooted device connected and click `Read from ADB`. gotohp will read the account's `lstBindingKeyAlias` from Android AccountManager and save it into the credential.

#### Troubleshooting

- **No Auth Request Intercepted**
  1. Log out of your Google account.
  2. Log in again.
  3. Try `Android App via Frida` interception method in HTTP Toolkit.
- **Token binding key not found**
  1. Make sure the same Google account is present on the connected device.
  2. Make sure root is available to ADB.

</details>

## Build

Build the CLI from the repository root with Go 1.27. On Windows, run `go build -tags cli -o gotohp-cli.exe .`. On macOS or Linux, run `go build -tags cli -o gotohp-cli .`.

For the desktop app, install Node.js 24 and the native prerequisites for your platform. Put Go's binary directory on your PATH, then run the following commands from the repository root. [Wails installation guide](https://v3.wails.io/getting-started/installation/)

```shell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.21
wails3 build
```

The desktop build writes the executable to `bin/`. On Linux, you can also build and run gotohp with Nix. [Nix instructions](nix/README.md)

Pushing a version tag such as `v1.2.3` starts the release builds. To enable automatic release pull requests, set `RELEASE_PLEASE_TOKEN` with write access to repository contents and pull requests.
