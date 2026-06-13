import type { AppSettings, Camera, CameraInput, MotionEvent, Recording } from "./types";
import { clearToken, getToken, withTokenParam } from "./auth";

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((init?.headers as Record<string, string>) || {}),
  };
  const tok = getToken();
  if (tok && !headers["Authorization"]) {
    headers["Authorization"] = `Bearer ${tok}`;
  }
  const res = await fetch(path, { ...init, headers });
  if (res.status === 401) {
    // Our stored token has gone stale (server restart, revocation). Drop it
    // and reload so the AuthGate renders the login screen.
    clearToken();
    window.location.reload();
    // Throw so callers don't try to consume the body.
    throw new Error("unauthorized");
  }
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const api = {
  startPairing: () =>
    req<{ code: string; expires_at: string }>("/api/pair/start", { method: "POST" }),
  listCameras: () => req<Camera[]>("/api/cameras"),
  getCamera: (id: number) => req<Camera>(`/api/cameras/${id}`),
  createCamera: (c: CameraInput) =>
    req<Camera>("/api/cameras", { method: "POST", body: JSON.stringify(c) }),
  updateCamera: (id: number, c: CameraInput) =>
    req<Camera>(`/api/cameras/${id}`, { method: "PUT", body: JSON.stringify(c) }),
  deleteCamera: (id: number) =>
    req<void>(`/api/cameras/${id}`, { method: "DELETE" }),
  listEvents: (params: {
    camera_id?: number;
    limit?: number;
    before?: string;
    codes?: string;
  } = {}) => {
    const q = new URLSearchParams();
    if (params.camera_id) q.set("camera_id", String(params.camera_id));
    if (params.limit) q.set("limit", String(params.limit));
    if (params.before) q.set("before", params.before);
    if (params.codes) q.set("codes", params.codes);
    const qs = q.toString();
    return req<MotionEvent[]>(`/api/events${qs ? "?" + qs : ""}`);
  },
  listRecordings: (params: {
    camera_id?: number;
    event_id?: number;
    limit?: number;
    before?: string;
  } = {}) => {
    const q = new URLSearchParams();
    if (params.camera_id) q.set("camera_id", String(params.camera_id));
    if (params.event_id) q.set("event_id", String(params.event_id));
    if (params.limit) q.set("limit", String(params.limit));
    if (params.before) q.set("before", params.before);
    const qs = q.toString();
    return req<Recording[]>(`/api/recordings${qs ? "?" + qs : ""}`);
  },
  getSettings: () => req<AppSettings>("/api/settings"),
  updateSettings: (s: AppSettings) =>
    req<AppSettings>("/api/settings", { method: "PUT", body: JSON.stringify(s) }),
};

// login exchanges admin credentials for a per-session API token. The server
// also returns must_change_password=true on the very first login (or whenever
// the password is still the default), which the UI uses to force a change.
export async function login(
  username: string,
  password: string,
): Promise<{ api_token: string; must_change_password: boolean; username: string }> {
  const res = await fetch("/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return res.json();
}

// changeAdminPassword updates the admin password and clears the must-change
// flag server-side. The caller must already be authenticated.
export async function changeAdminPassword(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  await req<void>("/api/admin/password", {
    method: "POST",
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  });
}

// Resource URLs — appended with ?token= so they're usable from <img>, <video>,
// and EventSource (which can't set headers).
export const streamUrl = (cameraId: number) =>
  withTokenParam(`/api/cameras/${cameraId}/stream.mjpg`);
export const hlsUrl = (cameraId: number) =>
  withTokenParam(`/api/cameras/${cameraId}/hls/index.m3u8`);
export const snapshotUrl = (cameraId: number, cacheBust?: number) =>
  withTokenParam(
    `/api/cameras/${cameraId}/snapshot.jpg${cacheBust ? `?t=${cacheBust}` : ""}`,
  );
export const clipUrl = (recordingId: number) =>
  withTokenParam(`/api/recordings/${recordingId}/clip.mp4`);
export const eventStreamUrl = () => withTokenParam("/api/events/stream");
