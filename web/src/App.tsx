import { NavLink, Route, Routes } from "react-router-dom";
import Dashboard from "./pages/Dashboard";
import Cameras from "./pages/Cameras";
import CameraDetail from "./pages/CameraDetail";
import Events from "./pages/Events";

export default function App() {
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
      </header>
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
