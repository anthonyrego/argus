import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api";
import type { Camera, MotionEvent } from "../types";

export default function Events() {
  const [events, setEvents] = useState<MotionEvent[]>([]);
  const [cams, setCams] = useState<Camera[]>([]);
  const [filterCam, setFilterCam] = useState<number | "">("");
  const [err, setErr] = useState<string | null>(null);

  async function load() {
    try {
      const list = await api.listEvents({
        limit: 200,
        camera_id: filterCam || undefined,
      });
      setEvents(list);
    } catch (e) {
      setErr(String((e as Error).message));
    }
  }

  useEffect(() => {
    api.listCameras().then(setCams).catch(() => {});
  }, []);

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterCam]);

  useEffect(() => {
    const es = new EventSource("/api/events/stream");
    es.addEventListener("motion", (e) => {
      try {
        const ev = JSON.parse((e as MessageEvent).data) as MotionEvent;
        if (!filterCam || ev.camera_id === filterCam) {
          setEvents((cur) => [ev, ...cur].slice(0, 500));
        }
      } catch {
        /* ignore */
      }
    });
    return () => es.close();
  }, [filterCam]);

  return (
    <div>
      <div className="row" style={{ marginBottom: 16 }}>
        <h2 style={{ margin: 0, fontSize: 18 }}>Motion events</h2>
        <div className="spacer" />
        <span className="muted" style={{ fontSize: 12 }}><span className="live-dot" />live</span>
        <select
          style={{ width: 220 }}
          value={filterCam}
          onChange={(e) => setFilterCam(e.target.value ? Number(e.target.value) : "")}
        >
          <option value="">All cameras</option>
          {cams.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
      </div>

      {err && <div className="error">{err}</div>}

      {events.length === 0 ? (
        <div className="empty">No events yet — they'll appear here when cameras report motion.</div>
      ) : (
        <table>
          <thead>
            <tr>
              <th style={{ width: 180 }}>Time</th>
              <th style={{ width: 160 }}>Camera</th>
              <th>Code</th>
              <th style={{ width: 80 }}>Action</th>
              <th>Data</th>
            </tr>
          </thead>
          <tbody>
            {events.map((e) => <EventRow key={e.id} ev={e} showCamera />)}
          </tbody>
        </table>
      )}
    </div>
  );
}

export function EventRow({ ev, showCamera }: { ev: MotionEvent; showCamera: boolean }) {
  const time = useMemo(() => new Date(ev.occurred_at).toLocaleString(), [ev.occurred_at]);
  const action = ev.action.toLowerCase();
  return (
    <tr>
      <td className="muted">{time}</td>
      {showCamera && (
        <td>
          <Link to={`/cameras/${ev.camera_id}`}>{ev.camera_name || `#${ev.camera_id}`}</Link>
        </td>
      )}
      <td><span className="badge code">{ev.code}</span></td>
      <td><span className={"badge " + action}>{ev.action}</span></td>
      <td className="muted" style={{ fontFamily: "ui-monospace, monospace", fontSize: 11 }}>
        {ev.data_json && ev.data_json.length > 80
          ? ev.data_json.slice(0, 80) + "…"
          : ev.data_json || ""}
      </td>
    </tr>
  );
}
