import SwiftUI

// LiveView shows a grid of enabled cameras. Each tile polls /snapshot.jpg
// once a second (the Dahua sub-stream snapshot is small and cheap). Tap a
// tile → CameraDetailView with the HLS feed.
struct LiveView: View {
    @EnvironmentObject private var state: AppState
    @State private var cameras: [Camera] = []
    @State private var errorText: String?

    private let columns = [GridItem(.adaptive(minimum: 160), spacing: 12)]

    var body: some View {
        NavigationStack {
            Group {
                if let err = errorText {
                    ContentUnavailableView(
                        "Failed to load",
                        systemImage: "exclamationmark.triangle",
                        description: Text(err)
                    )
                } else if enabledCameras.isEmpty {
                    ContentUnavailableView("No enabled cameras", systemImage: "video.slash")
                } else {
                    ScrollView {
                        LazyVGrid(columns: columns, spacing: 12) {
                            ForEach(enabledCameras) { cam in
                                NavigationLink(value: cam) { tile(for: cam) }
                                    .buttonStyle(.plain)
                            }
                        }
                        .padding(12)
                    }
                }
            }
            .navigationTitle("Live")
            .navigationDestination(for: Camera.self) { CameraDetailView(camera: $0) }
            .task { await load() }
            .refreshable { await load() }
        }
    }

    private var enabledCameras: [Camera] {
        cameras.filter(\.enabled)
    }

    @ViewBuilder
    private func tile(for cam: Camera) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            if let client = state.client {
                AuthenticatedImage(
                    url: client.snapshotURL(cam.id),
                    apiToken: state.apiToken ?? "",
                    interval: 1
                )
                .aspectRatio(16 / 9, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: 8))
            }
            Text(cam.name)
                .font(.subheadline.weight(.semibold))
                .lineLimit(1)
            Text(cam.host)
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
    }

    private func load() async {
        guard let client = state.client else { return }
        do {
            cameras = try await client.listCameras()
            errorText = nil
        } catch {
            errorText = error.localizedDescription
        }
    }
}
