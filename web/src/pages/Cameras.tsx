import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api";
import type { Camera, CameraInput } from "../types";

const EMPTY: CameraInput = {
  name: "",
  host: "",
  username: "admin",
  password: "",
  enabled: true,
};

export default function Cameras() {
  const [cams, setCams] = useState<Camera[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [editing, setEditing] = useState<Camera | "new" | null>(null);

  async function refresh() {
    try {
      setCams(await api.listCameras());
    } catch (e) {
      setErr(String((e as Error).message));
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  async function remove(c: Camera) {
    if (!confirm(`Delete camera "${c.name}"? This also removes its event history.`)) return;
    try {
      await api.deleteCamera(c.id);
      await refresh();
    } catch (e) {
      setErr(String((e as Error).message));
    }
  }

  return (
    <div>
      <div className="row" style={{ marginBottom: 16 }}>
        <h2 style={{ margin: 0, fontSize: 18 }}>Cameras</h2>
        <div className="spacer" />
        <button className="primary" onClick={() => setEditing("new")}>+ Add camera</button>
      </div>

      {err && <div className="error">{err}</div>}

      {!cams ? (
        <div className="muted">Loading…</div>
      ) : cams.length === 0 ? (
        <div className="empty">No cameras yet. Add one to get started.</div>
      ) : (
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Host</th>
              <th>User</th>
              <th>Status</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {cams.map((c) => (
              <tr key={c.id}>
                <td><Link to={`/cameras/${c.id}`}>{c.name}</Link></td>
                <td className="muted">{c.host}</td>
                <td className="muted">{c.username}</td>
                <td>
                  <span className={"badge " + (c.enabled ? "start" : "stop")}>
                    {c.enabled ? "enabled" : "disabled"}
                  </span>
                </td>
                <td style={{ textAlign: "right" }}>
                  <button onClick={() => setEditing(c)}>Edit</button>{" "}
                  <button className="danger" onClick={() => remove(c)}>Delete</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {editing && (
        <CameraDialog
          initial={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null);
            await refresh();
          }}
        />
      )}
    </div>
  );
}

function CameraDialog({
  initial,
  onClose,
  onSaved,
}: {
  initial: Camera | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [form, setForm] = useState<CameraInput>(
    initial
      ? {
          name: initial.name,
          host: initial.host,
          username: initial.username,
          password: "",
          enabled: initial.enabled,
        }
      : EMPTY,
  );
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setErr(null);
    try {
      if (initial) {
        await api.updateCamera(initial.id, form);
      } else {
        await api.createCamera(form);
      }
      onSaved();
    } catch (e) {
      setErr(String((e as Error).message));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h2>{initial ? `Edit ${initial.name}` : "Add camera"}</h2>
        {err && <div className="error">{err}</div>}
        <div className="form-row">
          <label>Name</label>
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        </div>
        <div className="form-row">
          <label>Host (IP or IP:port)</label>
          <input
            value={form.host}
            placeholder="10.10.70.244"
            onChange={(e) => setForm({ ...form, host: e.target.value })}
          />
        </div>
        <div className="form-row">
          <label>Username</label>
          <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} />
        </div>
        <div className="form-row">
          <label>{initial ? "Password (leave blank to keep)" : "Password"}</label>
          <input
            type="password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
          />
        </div>
        <div className="form-row row">
          <input
            type="checkbox"
            id="enabled"
            style={{ width: "auto" }}
            checked={form.enabled}
            onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
          />
          <label htmlFor="enabled" style={{ marginBottom: 0 }}>
            Enabled — listen for motion events
          </label>
        </div>
        <div className="actions">
          <button onClick={onClose} disabled={busy}>Cancel</button>
          <button className="primary" onClick={save} disabled={busy || !form.name || !form.host}>
            {busy ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
