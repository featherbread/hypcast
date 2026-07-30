import React from "react";

import { useWebRTC } from "../WebRTC";
import rpc from "../rpc";

import "./index.scss";

import Header from "./Header";
import ChannelSelector from "./ChannelSelector";
import { useTunerStatus } from "../TunerStatus";

export default function App() {
  const webRTC = useWebRTC();
  const tunerStatus = useTunerStatus();

  const selectedChannel =
    tunerStatus.Connection === "Connected" && tunerStatus.State !== "Stopped"
      ? tunerStatus.ChannelName
      : undefined;

  return (
    <div className="AppContainer">
      <PageTitle />
      <Header />
      <ChannelSelector
        selected={selectedChannel}
        onTune={(ch) => rpc("tune", { ChannelName: ch }).catch(console.error)}
      />
      <VideoPlayer stream={webRTC.MediaStream} />
    </div>
  );
}

function PageTitle() {
  const tunerStatus = useTunerStatus();
  const titleText =
    tunerStatus.Connection === "Connected" && tunerStatus.State !== "Stopped"
      ? `${tunerStatus.ChannelName} | Hypcast`
      : "Hypcast";

  return <title>{titleText}</title>;
}

function VideoPlayer({ stream }: { stream: undefined | MediaStream }) {
  const videoElement = React.useRef<null | HTMLVideoElement>(null);

  React.useEffect(() => {
    if (videoElement.current !== null) {
      videoElement.current.srcObject = stream ?? null;
    }
  }, [stream]);

  // eslint-disable jsx-a11y/media-has-caption
  // Lack of closed caption support is a longstanding deficiency in Hypcast.
  // After experimenting with several approaches in GStreamer, I'm ashamed to
  // say that I have yet to identify one that works reliably and consistently.
  // Also, it is not clear that the eventual implementation of closed captions
  // will involve WebVTT, which is what this rule actually looks for (through
  // the presence of a <track> element), so it may need to remain disabled even
  // after closed caption support is in place.
  return (
    <main className="VideoPlayer">
      <video
        style={{ display: stream === undefined ? "none" : undefined }}
        ref={videoElement}
        autoPlay
        controls
      />
    </main>
  );
}
