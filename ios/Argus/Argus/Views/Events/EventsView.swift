import SwiftUI

// EventsView is a chronological list of motion events. Tap → detail screen
// with the clip (or "recording in progress" if it's still being recorded).
// Subscribes to /api/events/stream while visible so new events appear without
// a refresh, and so the recordingsByEventID map updates when a recording
// finishes (covering the user-tapped-the-notification-immediately case).
struct EventsView: View {
    @EnvironmentObject private var state: AppState
    @State private var events: [MotionEvent] = []
    @State private var recordingsByEventID: [Int64: Recording] = [:]
    @State private var errorText: String?

    var body: some View {
        NavigationStack {
            Group {
                if let err = errorText, events.isEmpty {
                    ContentUnavailableView(
                        "Failed to load",
                        systemImage: "exclamationmark.triangle",
                        description: Text(err)
                    )
                } else if events.isEmpty {
                    ContentUnavailableView("No events yet", systemImage: "bell.slash")
                } else {
                    List(events) { ev in
                        NavigationLink(value: ev) {
                            EventRow(event: ev, hasClip: recordingsByEventID[ev.id] != nil)
                        }
                    }
                    .listStyle(.plain)
                }
            }
            .navigationTitle("Events")
            .navigationDestination(for: MotionEvent.self) { ev in
                EventDetailView(event: ev, initialRecording: recordingsByEventID[ev.id])
            }
            .task { await load() }
            .task { await listenForUpdates() }
            .refreshable { await load() }
        }
    }

    private func load() async {
        guard let client = state.client else { return }
        do {
            let evs = try await client.listEvents(limit: 200)
            events = evs
            // Pull recent recordings to populate the "has clip" markers.
            let recs = try await client.listRecordings(limit: 200)
            var map: [Int64: Recording] = [:]
            for r in recs {
                if let evID = r.eventID { map[evID] = r }
            }
            recordingsByEventID = map
            errorText = nil
        } catch {
            errorText = error.localizedDescription
        }
    }

    private func listenForUpdates() async {
        guard let url = state.baseURL, let token = state.apiToken else { return }
        let stream = EventStream(baseURL: url, apiToken: token)
        for await msg in await stream.messages() {
            switch msg {
            case .motion(let ev):
                if !events.contains(where: { $0.id == ev.id }) {
                    events.insert(ev, at: 0)
                }
            case .recording(let rec):
                if let evID = rec.eventID {
                    recordingsByEventID[evID] = rec
                }
            }
        }
    }
}

private struct EventRow: View {
    let event: MotionEvent
    let hasClip: Bool

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: hasClip ? "play.rectangle.fill" : "bell.fill")
                .foregroundStyle(hasClip ? .blue : .secondary)
                .imageScale(.large)
                .frame(width: 28)
            VStack(alignment: .leading, spacing: 2) {
                HStack {
                    Text(event.cameraName ?? "Camera \(event.cameraID)")
                        .font(.subheadline.weight(.semibold))
                    Spacer()
                    Text(event.occurredAt, style: .time)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Text(humanReadable(code: event.code))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 4)
    }

    private func humanReadable(code: String) -> String {
        switch code {
        case "SmartMotionHuman": return "Person detected"
        case "SmartMotionVehicle": return "Vehicle detected"
        case "CrossLineDetection": return "Crossed line"
        case "CrossRegionDetection": return "Entered region"
        case "VideoMotion": return "Motion"
        default: return code
        }
    }
}
