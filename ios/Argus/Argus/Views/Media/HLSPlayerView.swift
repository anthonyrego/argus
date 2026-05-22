import AVKit
import SwiftUI

// HLSPlayerView wraps AVPlayer for the camera's live HLS feed. The URL
// already carries the API token in the query string (see APIClient.hlsURL),
// and the server rewrites m3u8 segment URIs to propagate it so segment
// requests authenticate too.
struct HLSPlayerView: View {
    let url: URL

    @State private var player: AVPlayer?

    var body: some View {
        VideoPlayer(player: player)
            .aspectRatio(16 / 9, contentMode: .fit)
            .background(Color.black)
            .onAppear {
                if player == nil {
                    let p = AVPlayer(url: url)
                    p.allowsExternalPlayback = true
                    p.play()
                    player = p
                }
            }
            .onDisappear {
                player?.pause()
                player = nil
            }
    }
}
