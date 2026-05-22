import SwiftUI

struct RecordingDetailView: View {
    @EnvironmentObject private var state: AppState
    let recording: Recording

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if let client = state.client {
                    ClipPlayerView(url: client.clipURL(recording.id))
                        .clipShape(RoundedRectangle(cornerRadius: 12))
                }
                VStack(alignment: .leading, spacing: 8) {
                    row("Camera", value: recording.cameraName ?? "Camera \(recording.cameraID)")
                    row("When", value: recording.startedAt.formatted(date: .abbreviated, time: .standard))
                    row("Length", value: String(format: "%.1fs", Double(recording.durationMs) / 1000))
                    row("Size", value: ByteCountFormatter.string(fromByteCount: recording.sizeBytes, countStyle: .file))
                }
                .padding()
                .background(Color(.secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 12))
            }
            .padding(.horizontal)
        }
        .navigationTitle("Clip")
        .navigationBarTitleDisplayMode(.inline)
    }

    private func row(_ key: String, value: String) -> some View {
        HStack {
            Text(key).foregroundStyle(.secondary)
            Spacer()
            Text(value)
        }
        .font(.callout)
    }
}
