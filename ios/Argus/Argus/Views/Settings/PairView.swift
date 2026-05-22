import SwiftUI
import UIKit
import UserNotifications

// PairView completes registration by exchanging a 6-digit code (generated
// in the web UI's "Pair phone" button) for a per-device API token.
struct PairView: View {
    @EnvironmentObject private var state: AppState
    @State private var code: String = ""
    @State private var deviceName: String = UIDevice.current.name
    @State private var errorText: String?
    @State private var busy = false

    var body: some View {
        NavigationStack {
            Form {
                Section(header: Text("Server")) {
                    LabeledContent("URL", value: state.baseURL?.absoluteString ?? "—")
                        .font(.callout)
                    Button("Use a different server") {
                        state.forgetServer()
                    }
                    .foregroundStyle(.red)
                }
                Section(
                    header: Text("Pairing code"),
                    footer: Text("In the argus web UI, click 'Pair phone' to generate a 6-digit code.")
                ) {
                    TextField("123456", text: $code)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.numberPad)
                        .font(.system(.title2, design: .monospaced))
                        .multilineTextAlignment(.center)
                }
                Section(header: Text("This device's name")) {
                    TextField("Tony's iPhone", text: $deviceName)
                }
                if let err = errorText {
                    Section { Text(err).foregroundStyle(.red) }
                }
                Section {
                    Button {
                        Task { await submit() }
                    } label: {
                        if busy { ProgressView() } else { Text("Pair") }
                    }
                    .disabled(busy || code.count != 6)
                }
            }
            .navigationTitle("Pair this phone")
        }
    }

    private func submit() async {
        guard let url = state.baseURL else { return }
        errorText = nil
        busy = true
        defer { busy = false }

        // Ask for notification permission so the APNs token can ride along
        // with the pairing request — saves a follow-up PUT in the happy path.
        let granted = try? await UNUserNotificationCenter.current()
            .requestAuthorization(options: [.alert, .badge, .sound])
        if granted == true {
            await MainActor.run {
                UIApplication.shared.registerForRemoteNotifications()
            }
        }
        let apns = await waitForAPNsToken(timeout: 3.0)

        do {
            let device = try await completePairing(
                baseURL: url,
                code: code.trimmingCharacters(in: .whitespaces),
                name: deviceName,
                apnsToken: apns
            )
            guard let token = device.apiToken else {
                errorText = "Server didn't return an api_token"
                return
            }
            state.adoptToken(token)
        } catch {
            errorText = error.localizedDescription
        }
    }

    private func waitForAPNsToken(timeout seconds: TimeInterval) async -> String? {
        let deadline = Date().addingTimeInterval(seconds)
        while Date() < deadline {
            if let t = state.apnsDeviceToken { return t }
            try? await Task.sleep(nanoseconds: 100_000_000)
        }
        return state.apnsDeviceToken
    }
}
