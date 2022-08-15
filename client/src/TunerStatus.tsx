import React from "react";

import PersistentSocket from "./PersistentSocket";

type TunerStatus =
  | { State: "Starting" | "Playing"; ChannelName: string }
  | { State: "Stopped"; Error: undefined | string };

export type Status =
  | { Connection: "Disconnected" | "Connecting" }
  | ({ Connection: "Connected" } & TunerStatus);

const Context = React.createContext<null | Status>(null);

export const useTunerStatus = (): Status => {
  const status = React.useContext(Context);
  if (status === null) {
    throw new Error("useTunerStatus must be used within <TunerStatusProvider>");
  }
  return status;
};

export const TunerStatusProvider = ({
  children,
}: {
  children: React.ReactNode;
}) => {
  const [status, setStatus] = React.useState<Status>({
    Connection: "Connecting",
  });

  React.useEffect(() => {
    const socket = new PersistentSocket(
      `ws://${window.location.host}/api/socket/tuner-status`,
    );

    let closed = false;
    const close = () => {
      if (closed) {
        return;
      }
      closed = true;
      socket.removeAllListeners("message");
      socket.removeAllListeners("close");
      socket.removeAllListeners("error");
      socket.close();
    };

    socket.on("message", (evt) => {
      const status: TunerStatus = JSON.parse(evt.data);
      console.log("Received tuner status", status);
      setStatus({ Connection: "Connected", ...status });
    });

    socket.on("close", () => {
      console.log("Tuner status socket closed");
      setStatus({ Connection: "Disconnected" });
      close();
    });
    socket.on("error", (evt) => {
      console.error("Tuner status socket error", evt);
      setStatus({ Connection: "Disconnected" });
      close();
    });

    return close;
  }, []);

  return <Context.Provider value={status}>{children}</Context.Provider>;
};
