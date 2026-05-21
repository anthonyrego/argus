import type { Camera, CameraInput, MotionEvent, Recording } from "./types";

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
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
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const api = {
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
};

export const streamUrl = (cameraId: number) => `/api/cameras/${cameraId}/stream.mjpg`;
export const hlsUrl = (cameraId: number) => `/api/cameras/${cameraId}/hls/index.m3u8`;
export const snapshotUrl = (cameraId: number, cacheBust?: number) =>
  `/api/cameras/${cameraId}/snapshot.jpg${cacheBust ? `?t=${cacheBust}` : ""}`;
export const clipUrl = (recordingId: number) => `/api/recordings/${recordingId}/clip.mp4`;
