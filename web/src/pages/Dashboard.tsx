import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, streamUrl } from "../api";
import type { Camera } from "../types";

export default function Dashboard() {
  const [cams, setCams] = useState<Camera[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    api.listCameras().then(setCams).catch((e) => setErr(String(e.message || e)));
  }, []);

  if (err) return <div className="error">{err}</div>;
  if (!cams) return <div className="muted">Loading…</div>;

  const enabled = cams.filter((c) => c.enabled);
  if (enabled.length === 0) {
    return (
      <div className="empty">
        <p>No enabled cameras yet.</p>
        <Link to="/cameras">Add a camera →</Link>
      </div>
    );
  }

  return (
    <div className="grid">
      {enabled.map((c) => (
        <div key={c.id} className="card">
          <div className="card-header">
            <Link to={`/cameras/${c.id}`} style={{ fontWeight: 500 }}>{c.name}</Link>
            <span className="muted" style={{ fontSize: 11 }}>{c.host}</span>
          </div>
          <img
            className="feed"
            src={streamUrl(c.id)}
            alt={c.name}
            onError={(e) => {
              (e.target as HTMLImageElement).style.opacity = "0.3";
            }}
          />
        </div>
      ))}
    </div>
  );
}
