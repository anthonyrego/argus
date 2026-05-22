import Foundation

// SharedConfig stores the small bits of state that the Notification Service
// Extension also needs to see: the argus base URL and the API token used to
// fetch the snapshot image. Lives in an App Group's shared UserDefaults so
// both targets can read it.
enum SharedConfig {
    static let appGroupID = "group.ai.getaide.argus"

    private static var defaults: UserDefaults {
        guard let d = UserDefaults(suiteName: appGroupID) else {
            return .standard
        }
        return d
    }

    private static let baseURLKey = "argus.base_url"
    private static let apiTokenKey = "argus.api_token"

    static var baseURL: URL? {
        get { defaults.string(forKey: baseURLKey).flatMap(URL.init(string:)) }
        set { defaults.set(newValue?.absoluteString, forKey: baseURLKey) }
    }

    static var apiToken: String? {
        get { defaults.string(forKey: apiTokenKey) }
        set { defaults.set(newValue, forKey: apiTokenKey) }
    }

    // clearToken forgets the per-device API token but keeps the base URL —
    // sign-out should not make the user retype the server URL.
    static func clearToken() {
        defaults.removeObject(forKey: apiTokenKey)
    }

    static func clear() {
        defaults.removeObject(forKey: baseURLKey)
        defaults.removeObject(forKey: apiTokenKey)
    }
}
