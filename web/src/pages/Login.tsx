import { useState } from "react";
import { login } from "../api";
import { setMustChangePassword, setToken } from "../auth";

// Login authenticates with admin credentials and stores the resulting
// per-session API token in localStorage. If the server reports
// must_change_password (default admin/admin or admin reset), we persist that
// flag so the AuthGate routes the user through a forced password change
// before the rest of the app renders.
export default function Login() {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      const r = await login(username.trim(), password);
      setToken(r.api_token);
      setMustChangePassword(r.must_change_password);
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
        <h2>Sign in to argus</h2>
        <p className="muted" style={{ fontSize: 12, marginTop: -10, marginBottom: 16 }}>
          Default credentials are <code>admin</code> / <code>admin</code>. You'll be asked to change the password right after first login.
        </p>
        {err && <div className="error">{err}</div>}
        <div className="form-row">
          <label>Username</label>
          <input
            autoFocus
            autoCapitalize="off"
            autoCorrect="off"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
          />
        </div>
        <div className="form-row">
          <label>Password</label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <div className="actions">
          <button type="submit" className="primary" disabled={busy || !username.trim() || !password}>
            {busy ? "Signing in…" : "Sign in"}
          </button>
        </div>
      </form>
    </div>
  );
}
