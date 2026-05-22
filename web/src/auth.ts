// Minimal token store for the browser UI. The token is a per-browser API
// token issued by POST /api/devices; we persist it in localStorage and inject
// it into every request — header for JSON, ?token= for img/video/EventSource.

const KEY = "argus.token";
const MUST_CHANGE_KEY = "argus.must_change_password";

export function getToken(): string | null {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

export function setToken(t: string): void {
  try {
    localStorage.setItem(KEY, t);
  } catch {
    /* ignore */
  }
}

export function clearToken(): void {
  try {
    localStorage.removeItem(KEY);
    localStorage.removeItem(MUST_CHANGE_KEY);
  } catch {
    /* ignore */
  }
}

// Tracks the server's must_change_password flag so the UI keeps gating on it
// even after a refresh — until the user actually changes their password.
export function getMustChangePassword(): boolean {
  try {
    return localStorage.getItem(MUST_CHANGE_KEY) === "1";
  } catch {
    return false;
  }
}

export function setMustChangePassword(v: boolean): void {
  try {
    if (v) localStorage.setItem(MUST_CHANGE_KEY, "1");
    else localStorage.removeItem(MUST_CHANGE_KEY);
  } catch {
    /* ignore */
  }
}

// withTokenParam appends ?token=<...> (or &token=) for resources that can't
// carry an Authorization header — <img src>, <video src>, EventSource.
export function withTokenParam(path: string): string {
  const t = getToken();
  if (!t) return path;
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}token=${encodeURIComponent(t)}`;
}
