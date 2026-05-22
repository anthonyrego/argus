import SwiftUI

// ServerURLView is the very first screen the user ever sees — collects the
// argus server URL (their tailnet hostname). After this is set once, the URL
// persists across sign-outs, so re-pairing on this same server only asks
// for the 6-digit code.
struct ServerURLView: View {
    @EnvironmentObject private var state: AppState
    @State private var urlText: String = ""
    @State private var errorText: String?

    var body: some View {
        NavigationStack {
            Form {
                Section(header: Text("Argus server")) {
                    TextField("https://argus.your-tailnet.ts.net", text: $urlText)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                }
                if let err = errorText {
                    Section { Text(err).foregroundStyle(.red) }
                }
                Section(footer: Text("You'll only need this once. Sign out keeps the URL — you'll just need a fresh 6-digit code to re-pair.")) {
                    Button("Continue") { submit() }
                        .disabled(urlText.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
            .navigationTitle("Connect to argus")
        }
    }

    private func submit() {
        let trimmed = urlText.trimmingCharacters(in: .whitespaces)
        guard let url = URL(string: trimmed), url.scheme == "http" || url.scheme == "https" else {
            errorText = "URL must start with http:// or https://"
            return
        }
        errorText = nil
        state.setBaseURL(url)
    }
}
