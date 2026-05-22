import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { api, eventStreamUrl, hlsUrl, streamUrl } from "../api";
import type { Camera, MotionEvent, Recording } from "../types";
import { EventRow } from "./Events";
import HlsPlayer from "../components/HlsPlayer";

type Mode = "sub" | "main";

export default function CameraDetail() {
  const { id } = useParams();
  const camId = Number(id);
  const [cam, setCam] = useState<Camera | null>(null);
  const [events, setEvents] = useState<MotionEvent[]>([]);
  const [recs, setRecs] = useState<Record<number, Recording>>({});
  const [err, setErr] = useState<string | null>(null);
  const [mode, setMode] = useState<Mode>("sub");

  async function loadRecordings() {
    try {
      const list = await api.listRecordings({ camera_id: camId, limit: 100 });
      const byEvent: Record<number, Recording> = {};
      for (const r of list) {
        if (r.event_id) byEvent[r.event_id] = r;
      }
      setRecs(byEvent);
    } catch {
      /* ignore */
    }
  }

  useEffect(() => {
    if (!camId) return;
    api.getCamera(camId).then(setCam).catch((e) => setErr(String(e.message)));
    api.listEvents({ camera_id: camId, limit: 50 }).then(setEvents).catch(() => {});
    loadRecordings();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [camId]);

  useEffect(() => {
    if (!camId) return;
    const es = new EventSource(eventStreamUrl());
    es.addEventListener("motion", (e) => {
      try {
        const ev = JSON.parse((e as MessageEvent).data) as MotionEvent;
        if (ev.camera_id !== camId) return;
        setEvents((cur) => [ev, ...cur].slice(0, 50));
        if (ev.action.toLowerCase() === "start") {
          setTimeout(loadRecordings, 12_000);
        }
      } catch {
        /* ignore */
      }
    });
    return () => es.close();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [camId]);

  if (err) return <div className="error">{err}</div>;
  if (!cam) return <div className="muted">Loading…</div>;

  return (
    <div>
      <div className="row" style={{ marginBottom: 16 }}>
        <Link to="/cameras" className="muted" style={{ fontSize: 13 }}>← Cameras</Link>
        <h2 style={{ margin: 0, fontSize: 18 }}>{cam.name}</h2>
        <span className="muted">{cam.host}</span>
        <div className="spacer" />
        <span className={"badge " + (cam.enabled ? "start" : "stop")}>
          {cam.enabled ? "enabled" : "disabled"}
        </span>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "minmax(0, 2fr) minmax(280px, 1fr)", gap: 16 }}>
        <div className="card">
          <div className="card-header">
            <span><span className="live-dot" />Live</span>
            <div className="row" style={{ gap: 0 }}>
              <SegBtn active={mode === "sub"} onClick={() => setMode("sub")} side="left">
                Sub (MJPEG)
              </SegBtn>
              <SegBtn active={mode === "main"} onClick={() => setMode("main")} side="right">
                Main (HLS)
              </SegBtn>
            </div>
          </div>
          {!cam.enabled ? (
            <div className="empty">Camera is disabled.</div>
          ) : mode === "sub" ? (
            <img className="feed" src={streamUrl(cam.id)} alt={cam.name} />
          ) : (
            <HlsPlayer className="feed" src={hlsUrl(cam.id)} />
          )}
        </div>

        <div className="card">
          <div className="card-header">Recent motion</div>
          {events.length === 0 ? (
            <div className="empty">No events yet.</div>
          ) : (
            <table>
              <tbody>
                {events.map((e) => (
                  <EventRow key={e.id} ev={e} showCamera={false} recording={recs[e.id]} />
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}

function SegBtn({
  active,
  onClick,
  side,
  children,
}: {
  active: boolean;
  onClick: () => void;
  side: "left" | "right";
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      style={{
        background: active ? "var(--accent)" : "var(--panel-2)",
        color: active ? "#fff" : "var(--text)",
        borderColor: active ? "var(--accent)" : "var(--border)",
        borderTopLeftRadius: side === "left" ? 6 : 0,
        borderBottomLeftRadius: side === "left" ? 6 : 0,
        borderTopRightRadius: side === "right" ? 6 : 0,
        borderBottomRightRadius: side === "right" ? 6 : 0,
        marginLeft: side === "right" ? -1 : 0,
        fontSize: 12,
        padding: "4px 10px",
      }}
    >
      {children}
    </button>
  );
}
