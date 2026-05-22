# Argus iOS app

Companion app for the [argus](../) server. Live camera grid, motion event
timeline, recordings library, and APNs push notifications. SwiftUI, iOS 17+.

## One-time Xcode setup

The Swift sources live here as plain files; the `.xcodeproj` is created in
Xcode (the project format isn't pleasant to hand-edit). About 10 minutes of
clicking.

### 1. Create the project

- Xcode → File → New → Project → **iOS · App**
- Product Name: **Argus**
- Team: (your paid Apple Developer account)
- Organization Identifier: **ai.getaide** (so bundle ID becomes `ai.getaide.argus`)
- Interface: **SwiftUI**
- Language: **Swift**
- Save the project at `ios/` (overwrite — Xcode will create `Argus.xcodeproj` next to the `Argus/` source folder).

### 2. Add the source files

In Xcode's Project Navigator: delete the default `ContentView.swift` and
`ArgusApp.swift` that Xcode created, then **right-click the Argus group →
Add Files to Argus…** and add the entire `Argus/` source folder (choose
"Create groups", not "Create folder references").

### 3. Add the Notification Service Extension target

- File → New → Target → **Notification Service Extension**
- Product Name: **ArgusNotificationService**
- After creation, delete the auto-generated `NotificationService.swift` in
  the new target and add the one from `ArgusNotificationService/` instead.

### 4. Capabilities (Argus target → Signing & Capabilities)

- **+ Capability → Push Notifications**
- **+ Capability → App Groups → +** → name it `group.ai.getaide.argus`
- **+ Capability → Background Modes →** check "Remote notifications"

Then on the **ArgusNotificationService** target:
- **+ Capability → App Groups →** check the same `group.ai.getaide.argus`

The App Group lets the NSE read the base URL and API token that the main app
writes to shared `UserDefaults`. Without it the NSE can't fetch the snapshot
image.

Pre-staged entitlements files are already in this directory:
- `Argus/Argus.entitlements`
- `ArgusNotificationService/ArgusNotificationService.entitlements`

After adding the capabilities, point each target's **Code Signing Entitlements**
build setting at the corresponding file (Build Settings → Signing → Code
Signing Entitlements). Or just let Xcode generate one and confirm it ends up
with the same keys — App Group `group.ai.getaide.argus` on both targets, and
`aps-environment = development` on the Argus target.

### 5. Deployment target

Both targets: **iOS 17.0**.

### 5b. App Transport Security (Tailscale-specific)

iOS's App Transport Security blocks plain HTTP by default. Tailscale's
`100.64/10` CGNAT range is **not** in iOS's automatic local-networking
allowlist (only `10/8`, `172.16/12`, `192.168/16`, and `.local`/`.home.arpa`
get a free pass), so reaching argus over `http://100.x.y.z:8080` or
`http://argus.tailnet.ts.net:8080` will be blocked unless you do one of:

**Option A (recommended): Tailscale HTTPS.** Run `tailscale cert
<host>.<tailnet>.ts.net` on the argus box once. Tailscale provisions a real
Let's Encrypt certificate and renews it automatically. Serve argus behind
that cert (or terminate TLS in front of it). The app then talks HTTPS, ATS
is happy, no Info.plist exception needed.

**Option B: ATS exception.** In the **Argus target → Info** tab, add:

```xml
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSAllowsArbitraryLoads</key>
    <true/>
</dict>
```

This is a broad exception; only worth doing for a personal app you control
end-to-end. It also applies to the NSE so the snapshot fetch works.

If you only need LAN access (no tailnet), `NSAllowsLocalNetworking = true`
covers `192.168.*` / `10.*` / `.local` without opening anything else.

### 6. APNs auth key on the server

Separate one-time step on the argus server side, not in Xcode:

1. developer.apple.com → Certificates, IDs & Profiles → Keys → **+**
2. Enable Apple Push Notifications service (APNs), download the `.p8` file
   (you only get it once — save it).
3. Note the Key ID (10 chars) and your Team ID (under Membership).
4. Drop the `.p8` somewhere readable by argus and fill in `config.yaml`:

   ```yaml
   apns:
     team_id: "ABCD123456"
     key_id: "ABCD123456"
     key_path: "/path/to/AuthKey_XYZ.p8"
     bundle_id: "ai.getaide.argus"
     environment: "development"   # match the build (sandbox for dev, production for TestFlight/App Store)
   ```

   "development" matches builds you install via Xcode. Switch to "production"
   for TestFlight / Ad Hoc / App Store builds.

## Run

1. Plug in your iPhone, select it as the run destination.
2. Build & Run.
3. First launch: the app shows the **Connect** screen.
   - Base URL: your argus tailnet URL, e.g. `https://argus.your-tailnet.ts.net`
     (or `http://argus.local:8080` on LAN).
   - Bootstrap token: paste the value from `auth.bootstrap_token` in
     `config.yaml`.
4. On submit, the app calls `POST /api/devices`, persists the returned
   `api_token` in Keychain, registers for remote notifications, and pushes
   the resulting APNs token via `PUT /api/devices/me`. From then on, all
   API calls use the per-device token.

## End-to-end push test

After the app is connected:

1. On the argus server, trigger any camera with motion (wave at it, or POST
   a fake event via SQL: `INSERT INTO events …`, then the recorder won't
   actually open a clip — easier to just wave at a camera).
2. Within ~2 seconds you should see a banner notification with the camera
   name and a snapshot preview attached.
3. If you see text-only with no image: NSE didn't run. Check Xcode's
   **Devices and Simulators → Open Console** while the push fires — NSE
   logs go there. Common causes: App Group ID mismatch, wrong base URL in
   SharedConfig, ATS blocking the snapshot fetch.

## Environment gotcha

`aps-environment` (in the app's entitlements) and `apns.environment` (in
argus's `config.yaml`) must match:

| Build type             | App entitlement | Server config |
|------------------------|-----------------|---------------|
| Xcode "Run on device"  | `development`   | `development` |
| TestFlight / Release   | `production`    | `production`  |

Push silently no-ops if these are misaligned — APNs accepts the request but
the token is bound to the other environment and won't deliver.

## File layout

When Xcode creates the project it nests the source under `Argus/Argus/`,
which is where the Swift files actually live:

```
ios/
├── Argus/                                  # Xcode project root
│   ├── Argus.xcodeproj
│   ├── Argus/                              # main app target source
│   │   ├── ArgusApp.swift                  # App entry + AppDelegate
│   │   ├── ContentView.swift               # TabView entry / auth gate
│   │   ├── AppState.swift                  # ObservableObject app state
│   │   ├── SharedConfig.swift              # App Group UserDefaults wrapper
│   │   ├── Keychain.swift                  # tiny Keychain wrapper
│   │   ├── Models.swift                    # Codable Camera/Event/Recording
│   │   ├── Argus.entitlements              # aps-environment + App Group
│   │   ├── API/
│   │   │   ├── Client.swift                # URLSession-based API client
│   │   │   └── EventStream.swift           # SSE reader
│   │   └── Views/
│   │       ├── Settings/SettingsView.swift
│   │       ├── Settings/RegisterView.swift # first-run bootstrap-token screen
│   │       ├── Events/EventsView.swift
│   │       ├── Library/LibraryView.swift
│   │       └── Live/LiveView.swift
│   ├── ArgusTests/                         # Xcode-generated, unused
│   └── ArgusUITests/                       # Xcode-generated, unused
└── ArgusNotificationService/               # staged here; copy contents in
    │                                         after adding the NSE target
    ├── ArgusNotificationService.entitlements
    └── NotificationService.swift
```

## After Xcode runs the "New Project" wizard

Xcode will create its own default `ArgusApp.swift`, `ContentView.swift`, and
`Item.swift` (SwiftData example) inside `Argus/Argus/`. The instructions
above have **already overwritten** the two it shares names with, and `Item.swift`
has been deleted from disk. In Xcode you'll see one **red broken reference**
to `Item.swift` — right-click it in the Project Navigator → **Delete** →
**Remove Reference**.

To get the other source files into the Xcode project:
1. In the Project Navigator, right-click the **Argus** group (the inner one)
   → **Add Files to "Argus"…**
2. Select `AppState.swift`, `Keychain.swift`, `Models.swift`,
   `SharedConfig.swift`, `Argus.entitlements`, and the `API/` and `Views/`
   folders.
3. Ensure **"Create groups"** is selected (not folder references) and that
   only the **Argus** target is checked under "Add to targets".

## Adding the Notification Service Extension

When you do **File → New → Target → Notification Service Extension**,
Xcode will create `ios/Argus/ArgusNotificationService/` with its own
`NotificationService.swift`. Replace its contents with the one staged at
`ios/ArgusNotificationService/NotificationService.swift`, and point its
Code Signing Entitlements at the staged
`ios/ArgusNotificationService/ArgusNotificationService.entitlements` (or
manually re-add the App Group `group.ai.getaide.argus` to whatever
entitlements file Xcode generated).
