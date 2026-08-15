import { EventEmitter } from "events";

export type ConnectionState =
  | { Status: "Disconnected" | "Connecting" | "Connected" }
  | { Status: "Error"; Error: Error };

type Message = { SDP: RTCSessionDescriptionInit };

// This declaration merging is unsafe, but intentional to represent the
// intended EventEmitter API to consumers.
// eslint-disable-next-line typescript/no-unsafe-declaration-merging
declare interface Backend {
  emit(event: "connectionchange", state: ConnectionState): boolean;
  on(
    event: "connectionchange",
    listener: (state: ConnectionState) => void,
  ): this;

  emit(event: "streamreceived", stream: MediaStream): boolean;
  on(event: "streamreceived", listener: (stream: MediaStream) => void): this;

  emit(event: "streamremoved"): boolean;
  on(event: "streamremoved", listener: () => void): this;
}

class Backend extends EventEmitter {
  #peerConnection: RTCPeerConnection;
  #webSocket: WebSocket;

  #connectionState: ConnectionState = { Status: "Connecting" };
  #mediaStream: undefined | MediaStream;

  constructor() {
    super();
    this.#peerConnection = new RTCPeerConnection();
    this.#webSocket = new WebSocket(
      `ws://${window.location.host}/api/socket/webrtc-peer`,
    );
    this.#setup();
  }

  get connectionState() {
    return this.#connectionState;
  }

  close() {
    this.#peerConnection.close();
    this.#webSocket.close();
  }

  #setup() {
    this.#webSocket.addEventListener("open", () => this.#handleSocketOpen());
    this.#webSocket.addEventListener("close", () => this.#handleSocketClose());
    this.#webSocket.addEventListener("message", (evt) =>
      this.#handleSocketMessage(evt),
    );
    this.#webSocket.addEventListener("error", (evt) =>
      this.#handleSocketError(evt),
    );

    this.#peerConnection.addEventListener("track", (evt) =>
      this.#handlePeerConnectionTrack(evt),
    );
    this.#peerConnection.addEventListener("connectionstatechange", (evt) =>
      console.log(
        "Connection state",
        this.#peerConnection.connectionState,
        evt,
      ),
    );
    this.#peerConnection.addEventListener("signalingstatechange", (evt) =>
      console.log("Signaling state", this.#peerConnection.signalingState, evt),
    );

    this.#peerConnection.addTransceiver("video", { direction: "recvonly" });
    this.#peerConnection.addTransceiver("audio", { direction: "recvonly" });
  }

  #handleSocketMessage(evt: MessageEvent) {
    const message: Message = JSON.parse(evt.data);
    console.log("Received WebRTC offer", message);
    this.#handleRTCOffer(message.SDP).catch(() => {});
  }

  async #handleRTCOffer(sdp: RTCSessionDescriptionInit) {
    console.log("Received remote description", sdp);
    this.#peerConnection.setRemoteDescription(sdp).catch(() => {});

    const answer = await this.#peerConnection.createAnswer();
    console.log("Created local description", answer);
    await this.#peerConnection.setLocalDescription(answer);

    this.#webSocket.send(JSON.stringify({ SDP: answer }));
  }

  #handleSocketOpen() {
    this.#connectionState = { Status: "Connected" };
    this.emit("connectionchange", this.#connectionState);
  }

  #handleSocketClose() {
    this.#connectionState = { Status: "Disconnected" };
    this.emit("connectionchange", this.#connectionState);
  }

  #handleSocketError(evt: Event) {
    console.log("RTC socket error", evt);
    this.#connectionState = {
      Status: "Error",
      Error: new Error("failed to connect RTC socket"),
    };
    this.emit("connectionchange", this.#connectionState);
  }

  #handlePeerConnectionTrack(evt: RTCTrackEvent) {
    console.log("RTC track", evt);

    if (evt.streams.length < 1) {
      return;
    }

    const stream = evt.streams[0];
    if (this.#mediaStream?.id === stream.id) {
      return;
    }

    this.#mediaStream = stream;
    stream.addEventListener("removetrack", () =>
      this.#handleMediaStreamRemoveTrack(stream),
    );

    this.emit("streamreceived", stream);
  }

  #handleMediaStreamRemoveTrack(stream: MediaStream) {
    console.log("Track removed from stream", stream);

    if (this.#mediaStream?.id !== stream.id) {
      return;
    }

    this.#mediaStream = undefined;
    this.emit("streamremoved");
  }
}

export default Backend;
