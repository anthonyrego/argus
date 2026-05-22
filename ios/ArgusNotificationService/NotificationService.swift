import UserNotifications
import UIKit

// NotificationService runs in its own short-lived process when a remote
// notification arrives with `mutable-content: 1` and decorates the
// notification before it's shown. Our job: read snapshot_path from the
// payload, fetch the JPEG from argus over the tailnet, and attach it as
// the lock-screen preview image.
//
// The system gives us up to ~30s. If we run out, serviceExtensionTimeWillExpire
// delivers whatever we've built so far (typically the plain text alert).

final class NotificationService: UNNotificationServiceExtension {
    private var contentHandler: ((UNNotificationContent) -> Void)?
    private var bestAttempt: UNMutableNotificationContent?

    override func didReceive(
        _ request: UNNotificationRequest,
        withContentHandler contentHandler: @escaping (UNNotificationContent) -> Void
    ) {
        self.contentHandler = contentHandler
        self.bestAttempt = (request.content.mutableCopy() as? UNMutableNotificationContent)
        guard let content = bestAttempt else {
            contentHandler(request.content)
            return
        }
        Task { await self.decorate(content: content) }
    }

    override func serviceExtensionTimeWillExpire() {
        if let content = bestAttempt, let handler = contentHandler {
            handler(content)
        }
    }

    // decorate fetches the snapshot referenced by `snapshot_path` and attaches
    // it. On any failure we fall back to the plain text notification.
    private func decorate(content: UNMutableNotificationContent) async {
        defer { contentHandler?(content) }

        // 1) Resolve the snapshot URL. The payload carries a relative path;
        //    we combine it with the base URL stored in the shared App Group.
        guard
            let baseURLString = sharedDefaults().string(forKey: "argus.base_url"),
            let baseURL = URL(string: baseURLString),
            let apiToken = sharedDefaults().string(forKey: "argus.api_token"),
            let snapshotPath = content.userInfo["snapshot_path"] as? String,
            let url = URL(string: snapshotPath, relativeTo: baseURL)?.absoluteURL
        else {
            return
        }

        // 2) Fetch the JPEG with the bearer token.
        var req = URLRequest(url: url)
        req.setValue("Bearer \(apiToken)", forHTTPHeaderField: "Authorization")
        req.timeoutInterval = 8

        do {
            let (data, resp) = try await URLSession.shared.data(for: req)
            guard
                let http = resp as? HTTPURLResponse,
                (200..<300).contains(http.statusCode)
            else { return }

            // 3) Persist to a temp file so we can hand the OS a file URL —
            //    UNNotificationAttachment doesn't accept in-memory data.
            let tmp = FileManager.default.temporaryDirectory
                .appendingPathComponent(UUID().uuidString + ".jpg")
            try data.write(to: tmp)
            let attachment = try UNNotificationAttachment(
                identifier: "snapshot",
                url: tmp,
                options: [UNNotificationAttachmentOptionsTypeHintKey: "public.jpeg"]
            )
            content.attachments = [attachment]
        } catch {
            // Swallow — the plain text alert will still show.
        }
    }

    private func sharedDefaults() -> UserDefaults {
        UserDefaults(suiteName: "group.ai.getaide.argus") ?? .standard
    }
}
