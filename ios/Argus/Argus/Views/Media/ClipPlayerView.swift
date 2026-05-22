import AVKit
import SwiftUI

// ClipPlayerView plays a single MP4 (motion clip). URL has the API token
// embedded via APIClient.clipURL.
struct ClipPlayerView: View {
    let url: URL
    var autoplay: Bool = true

    @State private var player: AVPlayer?

    var body: some View {
        VideoPlayer(player: player)
            .aspectRatio(16 / 9, contentMode: .fit)
            .background(Color.black)
            .onAppear {
                if player == nil {
                    let p = AVPlayer(url: url)
                    p.allowsExternalPlayback = true
                    if autoplay { p.play() }
                    player = p
                }
            }
            .onDisappear {
                player?.pause()
                player = nil
            }
    }
}
