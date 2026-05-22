import { useState } from "react";
import { NavLink, Route, Routes } from "react-router-dom";
import Dashboard from "./pages/Dashboard";
import Cameras from "./pages/Cameras";
import CameraDetail from "./pages/CameraDetail";
import Events from "./pages/Events";
import Login from "./pages/Login";
import ChangePassword from "./pages/ChangePassword";
import PairDeviceModal from "./components/PairDeviceModal";
import { clearToken, getMustChangePassword, getToken } from "./auth";

export default function App() {
  if (!getToken()) return <Login />;
  if (getMustChangePassword()) return <ChangePassword />;
  return <AppShell />;
}

function AppShell() {
  const [pairOpen, setPairOpen] = useState(false);
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
        </nav>
        <div className="spacer" />
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
        </Routes>
      </main>
    </div>
  );
}
