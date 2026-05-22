import SwiftUI

// EventDetailView shows the event metadata and, once available, the clip
// AVPlayer. Because we push on event *start*, the clip won't exist for the
// first 10-30 seconds after the event lands; the view polls listRecordings
// periodically until the clip surfaces (capped at ~60s).
struct EventDetailView: View {
    @EnvironmentObject private var state: AppState
    let event: MotionEvent
    var initialRecording: Recording? = nil

    @State private var recording: Recording?
    @State private var pollAttempts = 0
    @State private var snapshotKey = UUID()

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                clipSection
                infoSection
            }
            .padding(.horizontal)
        }
        .navigationTitle(event.cameraName ?? "Camera \(event.cameraID)")
        .navigationBarTitleDisplayMode(.inline)
        .task(id: event.id) {
            recording = initialRecording
            await pollUntilRecordingAppears()
        }
    }

    @ViewBuilder
    private var clipSection: some View {
        if let rec = recording, let client = state.client {
            ClipPlayerView(url: client.clipURL(rec.id))
                .clipShape(RoundedRectangle(cornerRadius: 12))
        } else if let client = state.client {
            // Live snapshot while the clip is still being recorded.
            ZStack {
                AuthenticatedImage(
                    url: client.snapshotURL(event.cameraID),
                    apiToken: state.apiToken ?? "",
                    interval: 1
                )
                VStack(spacing: 6) {
                    ProgressView().tint(.white)
                    Text("Recording in progress…")
                        .font(.caption)
                        .foregroundStyle(.white)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 4)
                        .background(.black.opacity(0.55))
                        .clipShape(Capsule())
                }
            }
            .aspectRatio(16 / 9, contentMode: .fit)
            .id(snapshotKey)
            .clipShape(RoundedRectangle(cornerRadius: 12))
        }
    }

    @ViewBuilder
    private var infoSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            row("Time", value: event.occurredAt.formatted(date: .abbreviated, time: .standard))
            row("Code", value: event.code)
            row("Action", value: event.action)
            if let rec = recording {
                row("Clip length", value: String(format: "%.1fs", Double(rec.durationMs) / 1000))
            }
        }
        .padding()
        .background(Color(.secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }

    private func row(_ key: String, value: String) -> some View {
        HStack {
            Text(key).foregroundStyle(.secondary)
            Spacer()
            Text(value)
        }
        .font(.callout)
    }

    private func pollUntilRecordingAppears() async {
        guard recording == nil else { return }
        guard let client = state.client else { return }
        // Poll every 2s up to ~60s (covers max_clip_sec defaults with slack).
        let maxAttempts = 30
        while !Task.isCancelled, pollAttempts < maxAttempts, recording == nil {
            do {
                let recs = try await client.listRecordings(eventID: event.id, limit: 1)
                if let r = recs.first {
                    recording = r
                    return
                }
            } catch {
                // Network blip — keep polling.
            }
            pollAttempts += 1
            try? await Task.sleep(nanoseconds: 2_000_000_000)
        }
    }
}
