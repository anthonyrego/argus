import { useState } from "react";
import { api } from "../api";
import type { AppSettings } from "../types";

// Settings hosts the global "home mode" switches. The current values live in
// AppShell so the header can reflect a muted state; this page renders from those
// props and pushes changes back up after a successful save.
export default function Settings({
  settings,
  onChange,
}: {
  settings: AppSettings | null;
  onChange: (s: AppSettings) => void;
}) {
  const [busy, setBusy] = useState<keyof AppSettings | null>(null);
  const [err, setErr] = useState<string | null>(null);

  async function set(patch: Partial<AppSettings>) {
    if (!settings) return;
    const key = Object.keys(patch)[0] as keyof AppSettings;
    const next = { ...settings, ...patch };
    setBusy(key);
    setErr(null);
    try {
      // Send the full object — an omitted field would read as false server-side.
      onChange(await api.updateSettings(next));
    } catch (e) {
      setErr(String((e as Error).message));
    } finally {
      setBusy(null);
    }
  }

  return (
    <div>
      <div className="row" style={{ marginBottom: 16 }}>
        <h2 style={{ margin: 0, fontSize: 18 }}>Settings</h2>
      </div>

      {err && <div className="error">{err}</div>}

      {!settings ? (
        <div className="muted">Loading…</div>
      ) : (
        <div className="card" style={{ maxWidth: 560 }}>
          <div className="card-header">
            <span>Home mode</span>
            {(!settings.recording_enabled || !settings.notifications_enabled) && (
              <span className="badge pulse">muted</span>
            )}
          </div>
          <div className="card-body">
            <ToggleRow
              label="Recording"
              hint="Record motion clips. Live view and the event feed keep working when off."
              checked={settings.recording_enabled}
              busy={busy === "recording_enabled"}
              onChange={(v) => set({ recording_enabled: v })}
            />
            <ToggleRow
              label="Notifications"
              hint="Push alerts to paired devices when a clip is recorded."
              checked={settings.notifications_enabled}
              busy={busy === "notifications_enabled"}
              onChange={(v) => set({ notifications_enabled: v })}
            />
            <p className="muted" style={{ fontSize: 12, margin: "12px 0 0", lineHeight: 1.5 }}>
              Turn both off when you're home to stop recording and silence alerts.
              Notifications fire when a clip is recorded, so with recording off you
              won't get pushes regardless of the notifications switch.
            </p>
          </div>
        </div>
      )}
    </div>
  );
}

function ToggleRow({
  label,
  hint,
  checked,
  busy,
  onChange,
}: {
  label: string;
  hint: string;
  checked: boolean;
  busy: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="row" style={{ alignItems: "flex-start", padding: "8px 0" }}>
      <label className="switch" style={{ marginTop: 2 }}>
        <input
          type="checkbox"
          checked={checked}
          disabled={busy}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span className="slider" />
      </label>
      <div>
        <div style={{ fontSize: 14 }}>{label}</div>
        <div className="muted" style={{ fontSize: 12 }}>{hint}</div>
      </div>
    </div>
  );
}
