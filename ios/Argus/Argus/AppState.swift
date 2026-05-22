import Combine
import Foundation
import SwiftUI

// AppState owns the connection identity (base URL + API token) and the
// currently-known APNs device token. Views observe it; the API client and
// EventStream read from it.
@MainActor
final class AppState: ObservableObject {
    @Published private(set) var baseURL: URL?
    @Published private(set) var apiToken: String?
    @Published var apnsDeviceToken: String? {
        didSet { syncAPNsTokenIfNeeded() }
    }

    var isAuthenticated: Bool { baseURL != nil && apiToken != nil }

    private static let tokenAccount = "api-token"

    init() {
        let storedToken = Keychain.get(Self.tokenAccount)
        self.baseURL = SharedConfig.baseURL
        self.apiToken = storedToken
        if let t = storedToken { SharedConfig.apiToken = t }
    }

    // setBaseURL persists the server URL without granting a token. Used in
    // the first leg of the two-step pairing flow.
    func setBaseURL(_ url: URL) {
        self.baseURL = url
        SharedConfig.baseURL = url
    }

    // adoptToken records a freshly-issued per-device API token. The base URL
    // must already be set.
    func adoptToken(_ apiToken: String) {
        self.apiToken = apiToken
        SharedConfig.apiToken = apiToken
        Keychain.set(apiToken, for: Self.tokenAccount)
    }

    // signOut clears the API token but keeps the base URL so the user only
    // has to re-pair (6 digits), not re-enter their tailnet hostname.
    func signOut() {
        self.apiToken = nil
        SharedConfig.clearToken()
        Keychain.remove(Self.tokenAccount)
    }

    // forgetServer wipes everything, including the base URL. Used when the
    // user wants to point the app at a different argus instance.
    func forgetServer() {
        self.baseURL = nil
        self.apiToken = nil
        SharedConfig.clear()
        Keychain.remove(Self.tokenAccount)
    }

    var client: APIClient? {
        guard let url = baseURL, let tok = apiToken else { return nil }
        return APIClient(baseURL: url, apiToken: tok)
    }

    private func syncAPNsTokenIfNeeded() {
        guard let client = client, let apns = apnsDeviceToken else { return }
        Task {
            do {
                try await client.updateMyDeviceAPNsToken(apns)
            } catch {
                print("argus: failed to sync APNs token:", error)
            }
        }
    }
}
