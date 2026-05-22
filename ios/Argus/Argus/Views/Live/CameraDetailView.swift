import SwiftUI

// CameraDetailView is the fullscreen-ish view shown when a Live tile is
// tapped. Plays the camera's HLS feed via AVPlayer.
struct CameraDetailView: View {
    @EnvironmentObject private var state: AppState
    let camera: Camera

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if let client = state.client {
                    HLSPlayerView(url: client.hlsURL(camera.id))
                }
                VStack(alignment: .leading, spacing: 8) {
                    LabeledContent("Host", value: camera.host)
                    LabeledContent("Status", value: camera.enabled ? "Enabled" : "Disabled")
                }
                .padding()
                .background(Color(.secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 12))
                .padding(.horizontal)
            }
        }
        .navigationTitle(camera.name)
        .navigationBarTitleDisplayMode(.inline)
    }
}
