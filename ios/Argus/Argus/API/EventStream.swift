import Foundation

// EventStream consumes argus's /api/events/stream SSE endpoint. Two event
// types arrive: "motion" (a MotionEvent JSON) and "recording" (a Recording
// JSON). The stream uses a long-lived URLSession data task with line parsing.
actor EventStream {
    enum Message {
        case motion(MotionEvent)
        case recording(Recording)
    }

    private let url: URL
    private let apiToken: String
    private var task: URLSessionDataTask?
    private var continuation: AsyncStream<Message>.Continuation?

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
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "bad date \(str)")
        }
        return d
    }()

    init(baseURL: URL, apiToken: String) {
        self.url = baseURL.appending(path: "/api/events/stream")
        self.apiToken = apiToken
    }

    func messages() -> AsyncStream<Message> {
        AsyncStream { continuation in
            self.continuation = continuation
            Task { await self.run() }
            continuation.onTermination = { [weak self] _ in
                Task { await self?.stop() }
            }
        }
    }

    private func run() async {
        var req = URLRequest(url: url)
        req.setValue("Bearer \(apiToken)", forHTTPHeaderField: "Authorization")
        req.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        req.timeoutInterval = 60 * 60

        do {
            let (bytes, resp) = try await URLSession.shared.bytes(for: req)
            guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
                continuation?.finish()
                return
            }
            var eventType = "message"
            var dataLines: [String] = []
            for try await line in bytes.lines {
                if line.isEmpty {
                    if !dataLines.isEmpty {
                        dispatch(event: eventType, data: dataLines.joined(separator: "\n"))
                    }
                    eventType = "message"
                    dataLines.removeAll()
                    continue
                }
                if line.hasPrefix(":") { continue }
                if line.hasPrefix("event:") {
                    eventType = String(line.dropFirst("event:".count)).trimmingCharacters(in: .whitespaces)
                } else if line.hasPrefix("data:") {
                    dataLines.append(String(line.dropFirst("data:".count)).trimmingCharacters(in: .whitespaces))
                }
            }
        } catch {
            // Caller can retry by calling messages() again on a new stream.
        }
        continuation?.finish()
    }

    private func dispatch(event: String, data: String) {
        guard let bytes = data.data(using: .utf8) else { return }
        switch event {
        case "motion":
            if let m = try? Self.decoder.decode(MotionEvent.self, from: bytes) {
                continuation?.yield(.motion(m))
            }
        case "recording":
            if let r = try? Self.decoder.decode(Recording.self, from: bytes) {
                continuation?.yield(.recording(r))
            }
        default:
            break
        }
    }

    private func stop() {
        task?.cancel()
        task = nil
        continuation?.finish()
    }
}
