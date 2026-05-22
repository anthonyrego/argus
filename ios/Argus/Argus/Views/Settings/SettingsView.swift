import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var state: AppState
    @State private var confirmSignOut = false

    var body: some View {
        NavigationStack {
            Form {
                Section("Server") {
                    LabeledContent("URL", value: state.baseURL?.absoluteString ?? "—")
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
}
