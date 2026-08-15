import { useWebRTC, State as WebRTCState } from "../WebRTC";
import { useTunerStatus, Status as TunerStatus } from "../TunerStatus";
import rpc from "../rpc";
import useConfig from "../useConfig";

export default function Header() {
  return (
    <header className="Header">
      <Title />
      <PowerButton />
      <StatusIndicator />
    </header>
  );
}

function Title() {
  return <h1>hypcast</h1>;
}

function PowerButton() {
  const tunerStatus = useTunerStatus();
  const channelNames = useConfig<string[]>("channels");

  const poweredOn =
    tunerStatus.Connection === "Connected" && tunerStatus.State !== "Stopped";

  const handleClick = () => {
    if (poweredOn) {
      rpc("stop").catch(console.error);
    } else if (Array.isArray(channelNames)) {
      rpc("tune", { ChannelName: channelNames[0] }).catch(console.error);
    }
  };

  return (
    <button
      className={`PowerButton ${poweredOn ? "PowerButton--Active" : ""}`}
      onClick={handleClick}
    >
      <svg
        viewBox="0 0 360 360"
        className="PowerButton__Icon"
        aria-label="Power"
      >
        <g>
          <path d="m265.57 72.483c-7.646-4.394-17.355-2.01-21.575 5.297-4.217 7.307-1.925 16.547 5.095 20.536l5.813 4.406c29.892 22.655 49.207 58.524 49.207 98.923 0 68.543-55.565 124.11-124.11 124.11-68.543 0-124.11-55.566-124.11-124.11 0-39.822 18.771-75.242 47.934-97.944l6.177-4.809c7.521-4.306 10.222-13.806 6.003-21.112s-13.923-9.693-21.566-5.303l-6.408 4.742c-38.083 28.184-62.784 73.409-62.784 124.43-0.004 85.46 69.282 154.75 154.75 154.75s154.75-69.287 154.75-154.76c0-51.011-24.696-96.232-62.772-124.42l-6.41-4.737z" />
          <path d="m195.32 162.49c0 9.103-6.895 16.549-15.323 16.549s-15.324-7.446-15.324-16.549v-142.34c0-9.102 6.896-16.549 15.324-16.549 8.429 0 15.323 7.447 15.323 16.549v142.34z" />
        </g>
      </svg>
    </button>
  );
}

function StatusIndicator() {
  const webRTC = useWebRTC();
  const tunerStatus = useTunerStatus();

  const indicatorActive =
    webRTC.Connection.Status === "Connected" &&
    tunerStatus.Connection === "Connected" &&
    tunerStatus.State === "Playing";

  return (
    <div className="StatusIndicator">
      <div
        className={`StatusIndicator__Dot ${
          indicatorActive ? "StatusIndicator__Dot--Active" : ""
        }`}
      ></div>
      <span className="StatusIndicator__Description">
        {statusString(webRTC, tunerStatus)}
      </span>
    </div>
  );
}

function statusString(webRTC: WebRTCState, tunerStatus: TunerStatus): string {
  if (webRTC.Connection.Status !== "Connected") {
    return webRTC.Connection.Status;
  }

  if (tunerStatus.Connection !== "Connected") {
    return tunerStatus.Connection;
  }

  if (tunerStatus.State === "Stopped") {
    if (tunerStatus.Error !== undefined) {
      return "Failed to Tune";
    }
    return "Powered Off";
  }

  if (tunerStatus.State === "Starting") {
    return `Tuning to ${tunerStatus.ChannelName}`;
  }

  if (tunerStatus.State === "Playing") {
    return `Watching ${tunerStatus.ChannelName}`;
  }

  return tunerStatus.State;
}
