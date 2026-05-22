import { useEffect, useState } from "react";
import { api } from "../api";

// PairDeviceModal asks the server for a one-time pairing code and displays
// it for the user to type into a phone. The code is single-use and expires
// after 5 minutes server-side.
export default function PairDeviceModal({ onClose }: { onClose: () => void }) {
  const [code, setCode] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState<Date | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [now, setNow] = useState(Date.now());

  async function fetchCode() {
    setErr(null);
    setCode(null);
    try {
      const r = await api.startPairing();
      setCode(r.code);
      setExpiresAt(new Date(r.expires_at));
    } catch (e) {
      setErr(String((e as Error).message));
    }
  }

  useEffect(() => {
    fetchCode();
  }, []);

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const remainingMs = expiresAt ? Math.max(0, expiresAt.getTime() - now) : 0;
  const remaining = Math.floor(remainingMs / 1000);
  const mm = Math.floor(remaining / 60).toString().padStart(2, "0");
  const ss = (remaining % 60).toString().padStart(2, "0");
  const expired = !!expiresAt && remainingMs === 0;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} style={{ minWidth: 360 }}>
        <h2>Pair a phone</h2>
        <p className="muted" style={{ fontSize: 12, marginTop: -10, marginBottom: 16 }}>
          Open argus on your phone and enter this 6-digit code.
        </p>
        {err && <div className="error">{err}</div>}
        {code && (
          <div
            style={{
              textAlign: "center",
              fontSize: 40,
              letterSpacing: "0.4em",
              fontFamily: "ui-monospace, monospace",
              fontWeight: 500,
              padding: "16px 0",
              color: expired ? "var(--text-dim)" : "var(--accent)",
              userSelect: "all",
            }}
          >
            {code}
          </div>
        )}
        {expiresAt && (
          <div style={{ textAlign: "center", fontSize: 12, color: "var(--text-dim)" }}>
            {expired ? "Expired" : `Expires in ${mm}:${ss}`}
          </div>
        )}
        <div className="actions">
          {expired ? (
            <button className="primary" onClick={fetchCode}>
              Generate new code
            </button>
          ) : (
            <button onClick={onClose}>Done</button>
          )}
        </div>
      </div>
    </div>
  );
}
