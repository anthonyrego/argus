import SwiftUI

// ContentView gates the rest of the app behind a two-step pairing flow.
// First launch (no URL): ServerURLView. URL set but no token: PairView.
// Fully authenticated: MainTabView.
struct ContentView: View {
    @EnvironmentObject private var state: AppState

    var body: some View {
        if state.isAuthenticated {
            MainTabView()
        } else if state.baseURL == nil {
            ServerURLView()
        } else {
            PairView()
        }
    }
}

struct MainTabView: View {
    var body: some View {
        TabView {
            LiveView()
                .tabItem { Label("Live", systemImage: "video.fill") }
            EventsView()
                .tabItem { Label("Events", systemImage: "bell.fill") }
            LibraryView()
                .tabItem { Label("Library", systemImage: "film.fill") }
            SettingsView()
                .tabItem { Label("Settings", systemImage: "gearshape.fill") }
        }
    }
}
