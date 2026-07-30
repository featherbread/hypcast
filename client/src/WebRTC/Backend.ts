import { EventEmitter } from "events";

export type ConnectionState =
  | { Status: "Disconnected" | "Connecting" | "Connected" }
  | { Status: "Error"; Error: Error };

type Message = { SDP: RTCSessionDescriptionInit };

// eslint-disable @typescript-eslint/no-unsafe-declaration-merging
// TODO: I need to figure out what's up with this one.

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
  private pc: RTCPeerConnection;
  private ws: WebSocket;

  private _connectionState: ConnectionState = { Status: "Connecting" };
  private _mediaStream: undefined | MediaStream;

  constructor() {
    super();
    this.pc = new RTCPeerConnection();
    this.ws = new WebSocket(
      `ws://${window.location.host}/api/socket/webrtc-peer`,
    );
    this.setup();
  }

  private setup() {
    this.ws.addEventListener("message", (evt) => this.handleSocketMessage(evt));
    this.ws.addEventListener("open", () => this.handleSocketOpen());
    this.ws.addEventListener("close", () => this.handleSocketClose());
    this.ws.addEventListener("error", (evt) => this.handleSocketError(evt));

    this.pc.addEventListener("track", (evt) =>
      this.handlePeerConnectionTrack(evt),
    );
    this.pc.addEventListener("connectionstatechange", (evt) =>
      console.log("Connection state", this.pc.connectionState, evt),
    );
    this.pc.addEventListener("signalingstatechange", (evt) =>
      console.log("Signaling state", this.pc.signalingState, evt),
    );

    this.pc.addTransceiver("video", { direction: "recvonly" });
    this.pc.addTransceiver("audio", { direction: "recvonly" });
  }

  get connectionState() {
    return this._connectionState;
  }

  close() {
    this.pc.close();
    this.ws.close();
  }

  private handleSocketMessage(evt: MessageEvent) {
    const message: Message = JSON.parse(evt.data);
    console.log("Received WebRTC offer", message);
    this.handleRTCOffer(message.SDP);
  }

  private async handleRTCOffer(sdp: RTCSessionDescriptionInit) {
    console.log("Received remote description", sdp);
    this.pc.setRemoteDescription(sdp);

    const answer = await this.pc.createAnswer();
    console.log("Created local description", answer);
    await this.pc.setLocalDescription(answer);

    this.ws.send(JSON.stringify({ SDP: answer }));
  }

  private handleSocketOpen() {
    this._connectionState = { Status: "Connected" };
    this.emit("connectionchange", this._connectionState);
  }

  private handleSocketClose() {
    this._connectionState = { Status: "Disconnected" };
    this.emit("connectionchange", this._connectionState);
  }

  private handleSocketError(evt: Event) {
    console.log("RTC socket error", evt);
    this._connectionState = {
      Status: "Error",
      Error: new Error(evt.toString()),
    };
    this.emit("connectionchange", this._connectionState);
  }

  private handlePeerConnectionTrack(evt: RTCTrackEvent) {
    console.log("RTC track", evt);

    if (evt.streams.length < 1) {
      return;
    }

    const stream = evt.streams[0];
    if (this._mediaStream && this._mediaStream.id === stream.id) {
      return;
    }

    this._mediaStream = stream;
    stream.addEventListener("removetrack", () =>
      this.handleMediaStreamRemoveTrack(stream),
    );

    this.emit("streamreceived", stream);
  }

  private handleMediaStreamRemoveTrack(stream: MediaStream) {
    console.log("Track removed from stream", stream);

    if (!this._mediaStream || this._mediaStream.id !== stream.id) {
      return;
    }

    this._mediaStream = undefined;
    this.emit("streamremoved");
  }
}

export default Backend;
