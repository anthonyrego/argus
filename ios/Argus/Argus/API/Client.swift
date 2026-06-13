import Foundation

// APIClient is a small URLSession-backed wrapper for argus's REST API. Every
// request carries the per-device bearer token. The base URL is whatever the
// user configured at registration time (tailnet hostname in practice).
struct APIClient {
    let baseURL: URL
    let apiToken: String

    private static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        let primary = ISO8601DateFormatter()
        primary.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let secondary = ISO8601DateFormatter()
        secondary.formatOptions = [.withInternetDateTime]
        d.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let str = try container.decode(String.self)
            if let dt = primary.date(from: str) { return dt }
            if let dt = secondary.date(from: str) { return dt }
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "unrecognized date: \(str)"
            )
        }
        return d
    }()

    private func request(_ path: String, method: String = "GET", body: Data? = nil) -> URLRequest {
        var url = baseURL
        url.append(path: path)
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("Bearer \(apiToken)", forHTTPHeaderField: "Authorization")
        if let body = body {
            req.httpBody = body
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        return req
    }

    private func send<T: Decodable>(_ req: URLRequest) async throws -> T {
        let (data, resp) = try await URLSession.shared.data(for: req)
        try Self.check(resp, data: data)
        return try Self.decoder.decode(T.self, from: data)
    }

    private func sendVoid(_ req: URLRequest) async throws {
        let (data, resp) = try await URLSession.shared.data(for: req)
        try Self.check(resp, data: data)
    }

    private static func check(_ resp: URLResponse, data: Data) throws {
        guard let http = resp as? HTTPURLResponse else { throw APIError.notHTTP }
        guard (200..<300).contains(http.statusCode) else {
            let msg = String(data: data, encoding: .utf8) ?? ""
            throw APIError.http(http.statusCode, msg)
        }
    }

    func listCameras() async throws -> [Camera] {
        try await send(request("/api/cameras"))
    }

    func listEvents(cameraID: Int64? = nil, limit: Int = 100, before: Date? = nil) async throws -> [MotionEvent] {
        var comps = URLComponents(url: baseURL.appending(path: "/api/events"), resolvingAgainstBaseURL: false)!
        var items: [URLQueryItem] = [.init(name: "limit", value: String(limit))]
        if let camID = cameraID { items.append(.init(name: "camera_id", value: String(camID))) }
        if let before = before {
            let fmt = ISO8601DateFormatter()
            fmt.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            items.append(.init(name: "before", value: fmt.string(from: before)))
        }
        comps.queryItems = items
        var req = URLRequest(url: comps.url!)
        req.setValue("Bearer \(apiToken)", forHTTPHeaderField: "Authorization")
        return try await send(req)
    }

    func listRecordings(cameraID: Int64? = nil, eventID: Int64? = nil, limit: Int = 100, before: Date? = nil) async throws -> [Recording] {
        var comps = URLComponents(url: baseURL.appending(path: "/api/recordings"), resolvingAgainstBaseURL: false)!
        var items: [URLQueryItem] = [.init(name: "limit", value: String(limit))]
        if let camID = cameraID { items.append(.init(name: "camera_id", value: String(camID))) }
        if let evID = eventID { items.append(.init(name: "event_id", value: String(evID))) }
        if let before = before {
            let fmt = ISO8601DateFormatter()
            fmt.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            items.append(.init(name: "before", value: fmt.string(from: before)))
        }
        comps.queryItems = items
        var req = URLRequest(url: comps.url!)
        req.setValue("Bearer \(apiToken)", forHTTPHeaderField: "Authorization")
        return try await send(req)
    }

    // Authenticated media URLs. AVPlayer and similar consumers can't set
    // headers on subrequests, so we embed the API token in the query string.
    // The server's auth middleware accepts ?token= as a fallback, and the
    // HLS handler rewrites m3u8 segment URIs to carry the same token.
    func clipURL(_ recordingID: Int64) -> URL {
        authenticated("/api/recordings/\(recordingID)/clip.mp4")
    }

    func snapshotURL(_ cameraID: Int64) -> URL {
        authenticated("/api/cameras/\(cameraID)/snapshot.jpg")
    }

    func hlsURL(_ cameraID: Int64) -> URL {
        authenticated("/api/cameras/\(cameraID)/hls/index.m3u8")
    }

    private func authenticated(_ path: String) -> URL {
        var comps = URLComponents(url: baseURL.appending(path: path), resolvingAgainstBaseURL: false)!
        var items = comps.queryItems ?? []
        items.append(URLQueryItem(name: "token", value: apiToken))
        comps.queryItems = items
        return comps.url!
    }

    func updateMyDeviceAPNsToken(_ apns: String) async throws {
        struct Body: Encodable { let name = ""; let apns_token: String }
        let body = try JSONEncoder().encode(Body(apns_token: apns))
        try await sendVoid(request("/api/devices/me", method: "PUT", body: body))
    }

    func getSettings() async throws -> AppSettings {
        try await send(request("/api/settings"))
    }

    // updateSettings replaces both global switches and returns the stored values.
    func updateSettings(_ settings: AppSettings) async throws -> AppSettings {
        let body = try JSONEncoder().encode(settings)
        return try await send(request("/api/settings", method: "PUT", body: body))
    }
}

enum APIError: Error, LocalizedError {
    case notHTTP
    case http(Int, String)

    var errorDescription: String? {
        switch self {
        case .notHTTP: return "not an HTTP response"
        case .http(let code, let msg): return "HTTP \(code): \(msg)"
        }
    }
}

// completePairing exchanges a 6-digit code (issued by the web UI's "Pair
// phone" button) for a per-device API token. Called unauthenticated; the
// code itself is the secret.
func completePairing(
    baseURL: URL,
    code: String,
    platform: String = "ios",
    name: String,
    apnsToken: String?
) async throws -> RegisteredDevice {
    var req = URLRequest(url: baseURL.appending(path: "/api/pair/complete"))
    req.httpMethod = "POST"
    req.setValue("application/json", forHTTPHeaderField: "Content-Type")
    struct Body: Encodable {
        let code: String
        let platform: String
        let name: String
        let apns_token: String
    }
    req.httpBody = try JSONEncoder().encode(Body(
        code: code,
        platform: platform,
        name: name,
        apns_token: apnsToken ?? ""
    ))
    let (data, resp) = try await URLSession.shared.data(for: req)
    guard let http = resp as? HTTPURLResponse else { throw APIError.notHTTP }
    guard (200..<300).contains(http.statusCode) else {
        let msg = String(data: data, encoding: .utf8) ?? ""
        throw APIError.http(http.statusCode, msg)
    }
    return try JSONDecoder().decode(RegisteredDevice.self, from: data)
}
