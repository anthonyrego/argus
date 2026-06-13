import { useEffect, useState } from "react";
import { NavLink, Route, Routes } from "react-router-dom";
import Dashboard from "./pages/Dashboard";
import Cameras from "./pages/Cameras";
import CameraDetail from "./pages/CameraDetail";
import Events from "./pages/Events";
import Settings from "./pages/Settings";
import Login from "./pages/Login";
import ChangePassword from "./pages/ChangePassword";
import PairDeviceModal from "./components/PairDeviceModal";
import { api } from "./api";
import type { AppSettings } from "./types";
import { clearToken, getMustChangePassword, getToken } from "./auth";

export default function App() {
  if (!getToken()) return <Login />;
  if (getMustChangePassword()) return <ChangePassword />;
  return <AppShell />;
}

function AppShell() {
  const [pairOpen, setPairOpen] = useState(false);
  const [settings, setSettings] = useState<AppSettings | null>(null);

  useEffect(() => {
    api.getSettings().then(setSettings).catch(() => {});
  }, []);

  const muted =
    settings && (!settings.recording_enabled || !settings.notifications_enabled);

  return (
    <div className="app">
      <header className="topbar">
        <h1>argus</h1>
        <nav>
          <NavLink to="/" end className={({ isActive }) => (isActive ? "active" : "")}>
            Live
          </NavLink>
          <NavLink to="/cameras" className={({ isActive }) => (isActive ? "active" : "")}>
            Cameras
          </NavLink>
          <NavLink to="/events" className={({ isActive }) => (isActive ? "active" : "")}>
            Events
          </NavLink>
          <NavLink to="/settings" className={({ isActive }) => (isActive ? "active" : "")}>
            Settings
          </NavLink>
        </nav>
        <div className="spacer" />
        {muted && (
          <NavLink to="/settings" className="badge pulse" style={{ textDecoration: "none" }}>
            home mode
          </NavLink>
        )}
        <span className="muted" style={{ fontSize: 12 }}>
          <span className="live-dot" />
          local
        </span>
        <button
          onClick={() => setPairOpen(true)}
          style={{ fontSize: 12, padding: "4px 10px" }}
          title="Pair a phone with argus"
        >
          Pair phone
        </button>
        <button
          onClick={() => {
            clearToken();
            window.location.reload();
          }}
          style={{ fontSize: 12, padding: "4px 10px" }}
          title="Forget this browser's token"
        >
          Sign out
        </button>
      </header>
      {pairOpen && <PairDeviceModal onClose={() => setPairOpen(false)} />}
      <main>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/cameras" element={<Cameras />} />
          <Route path="/cameras/:id" element={<CameraDetail />} />
          <Route path="/events" element={<Events />} />
          <Route
            path="/settings"
            element={<Settings settings={settings} onChange={setSettings} />}
          />
        </Routes>
      </main>
    </div>
  );
}
