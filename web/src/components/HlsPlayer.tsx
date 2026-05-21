import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";

type Props = {
  src: string;
  className?: string;
};

// HlsPlayer wraps a <video> element with hls.js (or native HLS on Safari).
// It treats the stream as live: low buffer, snap to live edge on stall.
export default function HlsPlayer({ src, className }: Props) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    setErr(null);

    let hls: Hls | null = null;

    if (Hls.isSupported()) {
      hls = new Hls({
        // Live tuning: keep latency low, recover from stalls aggressively.
        liveSyncDurationCount: 2,
        liveMaxLatencyDurationCount: 4,
        lowLatencyMode: true,
        maxBufferLength: 6,
        maxLiveSyncPlaybackRate: 1.5,
      });
      hls.loadSource(src);
      hls.attachMedia(video);
      hls.on(Hls.Events.ERROR, (_evt, data) => {
        if (data.fatal) {
          setErr(`${data.type}: ${data.details}`);
          // Try a single recover before giving up.
          if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
            hls?.startLoad();
          } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
            hls?.recoverMediaError();
          }
        }
      });
    } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = src;
    } else {
      setErr("HLS not supported in this browser");
    }

    return () => {
      if (hls) {
        hls.destroy();
      } else {
        video.removeAttribute("src");
        video.load();
      }
    };
  }, [src]);

  return (
    <div style={{ position: "relative" }}>
      <video
        ref={videoRef}
        className={className}
        autoPlay
        muted
        playsInline
        controls={false}
      />
      {err && (
        <div
          style={{
            position: "absolute",
            inset: 0,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            background: "rgba(0,0,0,0.6)",
            color: "var(--danger)",
            fontSize: 13,
          }}
        >
          {err}
        </div>
      )}
    </div>
  );
}
