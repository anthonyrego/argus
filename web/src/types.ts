export type Camera = {
  id: number;
  name: string;
  host: string;
  username: string;
  enabled: boolean;
  created_at: string;
};

export type CameraInput = {
  name: string;
  host: string;
  username: string;
  password: string;
  enabled: boolean;
};

export type MotionEvent = {
  id: number;
  camera_id: number;
  camera_name?: string;
  code: string;
  action: string;
  channel_idx: number;
  data_json?: string;
  occurred_at: string;
};

export type Recording = {
  id: number;
  camera_id: number;
  camera_name?: string;
  event_id?: number;
  started_at: string;
  ended_at: string;
  duration_ms: number;
  size_bytes: number;
  created_at: string;
};
