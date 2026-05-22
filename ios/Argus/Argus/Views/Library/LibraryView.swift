import SwiftUI

// Library tab: chronological list of saved clips. Tap → RecordingDetailView
// with AVPlayer.
struct LibraryView: View {
    @EnvironmentObject private var state: AppState
    @State private var recordings: [Recording] = []
    @State private var errorText: String?

    var body: some View {
        NavigationStack {
            Group {
                if let err = errorText, recordings.isEmpty {
                    ContentUnavailableView(
                        "Failed to load",
                        systemImage: "exclamationmark.triangle",
                        description: Text(err)
                    )
                } else if recordings.isEmpty {
                    ContentUnavailableView("No recordings yet", systemImage: "film")
                } else {
                    List(recordings) { rec in
                        NavigationLink(value: rec) { RecordingRow(recording: rec) }
                    }
                    .listStyle(.plain)
                }
            }
            .navigationTitle("Library")
            .navigationDestination(for: Recording.self) { RecordingDetailView(recording: $0) }
            .task { await load() }
            .refreshable { await load() }
        }
    }

    private func load() async {
        guard let client = state.client else { return }
        do {
            recordings = try await client.listRecordings(limit: 200)
            errorText = nil
        } catch {
            errorText = error.localizedDescription
        }
    }
}

private struct RecordingRow: View {
    let recording: Recording

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "play.rectangle.fill")
                .foregroundStyle(.blue)
                .imageScale(.large)
                .frame(width: 28)
            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(recording.cameraName ?? "Camera \(recording.cameraID)")
                        .font(.subheadline.weight(.semibold))
                    Spacer()
                    Text(recording.startedAt, style: .time)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                HStack(spacing: 6) {
                    Text(String(format: "%.1fs", Double(recording.durationMs) / 1000))
                    Text("·").foregroundStyle(.tertiary)
                    Text(ByteCountFormatter.string(fromByteCount: recording.sizeBytes, countStyle: .file))
                }
                .font(.caption)
                .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 4)
    }
}
