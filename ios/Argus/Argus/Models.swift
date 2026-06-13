import Foundation

struct Camera: Codable, Identifiable, Hashable {
    let id: Int64
    let name: String
    let host: String
    let username: String
    let enabled: Bool
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id, name, host, username, enabled
        case createdAt = "created_at"
    }
}

struct MotionEvent: Codable, Identifiable, Hashable {
    let id: Int64
    let cameraID: Int64
    let cameraName: String?
    let code: String
    let action: String
    let channelIdx: Int
    let dataJSON: String?
    let occurredAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case cameraID = "camera_id"
        case cameraName = "camera_name"
        case code, action
        case channelIdx = "channel_idx"
        case dataJSON = "data_json"
        case occurredAt = "occurred_at"
    }
}

struct Recording: Codable, Identifiable, Hashable {
    let id: Int64
    let cameraID: Int64
    let cameraName: String?
    let eventID: Int64?
    let startedAt: Date
    let endedAt: Date
    let durationMs: Int64
    let sizeBytes: Int64
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case cameraID = "camera_id"
        case cameraName = "camera_name"
        case eventID = "event_id"
        case startedAt = "started_at"
        case endedAt = "ended_at"
        case durationMs = "duration_ms"
        case sizeBytes = "size_bytes"
        case createdAt = "created_at"
    }
}

// AppSettings mirrors the server's global "home mode" switches.
struct AppSettings: Codable, Equatable {
    var recordingEnabled: Bool
    var notificationsEnabled: Bool

    enum CodingKeys: String, CodingKey {
        case recordingEnabled = "recording_enabled"
        case notificationsEnabled = "notifications_enabled"
    }
}

struct RegisteredDevice: Codable {
    let id: Int64
    let platform: String
    let name: String
    let apiToken: String?
    let hasAPNsToken: Bool
    let createdAt: String
    let lastSeenAt: String

    enum CodingKeys: String, CodingKey {
        case id, platform, name
        case apiToken = "api_token"
        case hasAPNsToken = "has_apns_token"
        case createdAt = "created_at"
        case lastSeenAt = "last_seen_at"
    }
}
