import { useState } from "react";
import { changeAdminPassword } from "../api";
import { setMustChangePassword } from "../auth";

// ChangePassword is shown immediately after a login where the server
// reported must_change_password=true. It's also reachable later as a normal
// settings screen by passing optional={true}.
export default function ChangePassword({ optional = false }: { optional?: boolean }) {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    if (next.length < 8) {
      setErr("New password must be at least 8 characters.");
      return;
    }
    if (next !== confirm) {
      setErr("Passwords don't match.");
      return;
    }
    setBusy(true);
    try {
      await changeAdminPassword(current, next);
      setMustChangePassword(false);
      window.location.replace("/");
    } catch (e) {
      setErr(String((e as Error).message));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "var(--bg)",
      }}
    >
      <form className="modal" onSubmit={submit} style={{ minWidth: 360 }}>
        <h2>{optional ? "Change password" : "Set a new password"}</h2>
        <p className="muted" style={{ fontSize: 12, marginTop: -10, marginBottom: 16 }}>
          {optional
            ? "Update the admin password used to sign in."
            : "Your current password is the default. Choose a new one before continuing."}
        </p>
        {err && <div className="error">{err}</div>}
        <div className="form-row">
          <label>Current password</label>
          <input
            type="password"
            autoFocus
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </div>
        <div className="form-row">
          <label>New password (8+ characters)</label>
          <input
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
          />
        </div>
        <div className="form-row">
          <label>Confirm new password</label>
          <input
            type="password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
        </div>
        <div className="actions">
          <button
            type="submit"
            className="primary"
            disabled={busy || !current || !next || !confirm}
          >
            {busy ? "Updating…" : "Update password"}
          </button>
        </div>
      </form>
    </div>
  );
}
