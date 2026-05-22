import SwiftUI

// AuthenticatedImage fetches an image from argus periodically using the
// per-device bearer token. Used for snapshot polling on the Live grid.
//
// SwiftUI's AsyncImage can't carry an Authorization header, and we'd rather
// not splatter the token into <img>-style URLs even though the server does
// accept ?token= — keeping the token in the header avoids logging/caching
// it in URLSession's URL cache.
struct AuthenticatedImage: View {
    let url: URL
    let apiToken: String
    /// Refresh interval. nil → fetch once.
    var interval: TimeInterval? = nil

    @State private var image: UIImage?
    @State private var failed = false

    var body: some View {
        ZStack {
            Color.black
            if let image = image {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFit()
            } else if failed {
                Image(systemName: "video.slash")
                    .foregroundStyle(.secondary)
                    .imageScale(.large)
            } else {
                ProgressView().tint(.white)
            }
        }
        .task(id: url) { await pollLoop() }
    }

    private func pollLoop() async {
        await fetch()
        guard let interval = interval, interval > 0 else { return }
        while !Task.isCancelled {
            try? await Task.sleep(nanoseconds: UInt64(interval * 1_000_000_000))
            if Task.isCancelled { return }
            await fetch()
        }
    }

    private func fetch() async {
        var req = URLRequest(url: url)
        req.setValue("Bearer \(apiToken)", forHTTPHeaderField: "Authorization")
        req.cachePolicy = .reloadIgnoringLocalCacheData
        do {
            let (data, resp) = try await URLSession.shared.data(for: req)
            guard
                let http = resp as? HTTPURLResponse,
                (200..<300).contains(http.statusCode),
                let img = UIImage(data: data)
            else {
                failed = image == nil
                return
            }
            failed = false
            image = img
        } catch {
            failed = image == nil
        }
    }
}
