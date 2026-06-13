import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var state: AppState
    @State private var confirmSignOut = false
    @State private var settings: AppSettings?
    @State private var settingsError: String?

    var body: some View {
        NavigationStack {
            Form {
                Section("Server") {
                    LabeledContent("URL", value: state.baseURL?.absoluteString ?? "—")
                }
                Section {
                    if settings != nil {
                        Toggle("Recording", isOn: toggle(\.recordingEnabled))
                        Toggle("Notifications", isOn: toggle(\.notificationsEnabled))
                    } else {
                        HStack {
                            Text("Loading…").foregroundStyle(.secondary)
                            Spacer()
                            ProgressView()
                        }
                    }
                    if let e = settingsError {
                        Text(e).font(.caption).foregroundStyle(.red)
                    }
                } header: {
                    Text("Home mode")
                } footer: {
                    Text("Turn both off when you're home. Notifications fire when a clip is recorded, so with recording off you won't get pushes regardless of the notifications switch.")
                }
                Section("Push notifications") {
                    LabeledContent("Status", value: state.apnsDeviceToken == nil ? "Not registered" : "Registered")
                    if let t = state.apnsDeviceToken {
                        LabeledContent("Token") {
                            Text(t.prefix(12) + "…")
                                .font(.system(.caption, design: .monospaced))
                        }
                    }
                }
                Section {
                    Button(role: .destructive) {
                        confirmSignOut = true
                    } label: {
                        Text("Sign out")
                    }
                    Button(role: .destructive) {
                        state.forgetServer()
                    } label: {
                        Text("Use a different server")
                    }
                }
            }
            .navigationTitle("Settings")
            .task { await loadSettings() }
            .confirmationDialog(
                "Sign out?",
                isPresented: $confirmSignOut,
                titleVisibility: .visible
            ) {
                Button("Sign out", role: .destructive) { state.signOut() }
                Button("Cancel", role: .cancel) { }
            } message: {
                Text("You'll need a fresh 6-digit pairing code to sign back in.")
            }
        }
    }

    // toggle binds a single switch: optimistically flips the local copy, then
    // pushes the full object to the server (an omitted field reads as false).
    private func toggle(_ keyPath: WritableKeyPath<AppSettings, Bool>) -> Binding<Bool> {
        Binding(
            get: { settings?[keyPath: keyPath] ?? false },
            set: { newValue in
                guard var s = settings else { return }
                s[keyPath: keyPath] = newValue
                settings = s
                Task { await saveSettings(s) }
            }
        )
    }

    private func loadSettings() async {
        guard let client = state.client else { return }
        do {
            settings = try await client.getSettings()
            settingsError = nil
        } catch {
            settingsError = "Couldn't load settings: \(error.localizedDescription)"
        }
    }

    private func saveSettings(_ s: AppSettings) async {
        guard let client = state.client else { return }
        do {
            settings = try await client.updateSettings(s)
            settingsError = nil
        } catch {
            settingsError = "Couldn't save: \(error.localizedDescription)"
            await loadSettings() // fall back to server truth
        }
    }
}
