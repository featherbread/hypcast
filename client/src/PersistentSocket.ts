import { EventEmitter } from "events";

class PersistentSocket extends EventEmitter {
  private url: string | URL;
  private protocols: string | string[] | undefined;

  private socket: WebSocket | undefined;
  private reconnectTimeout: ReturnType<typeof setTimeout> | undefined;

  constructor(url: string | URL, protocols?: string | string[] | undefined) {
    super();
    this.url = url;
    this.protocols = protocols;
    this.reinitialize();
  }

  send(data: string | ArrayBufferLike | Blob | ArrayBufferView) {
    if (this.socket !== undefined) {
      this.socket.send(data);
    } else {
      throw new Error("Attempted to send data on closed socket");
    }
  }

  close() {
    if (this.socket !== undefined) {
      this.socket.close();
    }
  }

  private reinitialize() {
    if (this.socket) {
      this.socket.removeEventListener("open", this.handleOpen.bind(this));
      this.socket.removeEventListener("message", this.handleMessage.bind(this));
      this.socket.removeEventListener("close", this.handleClose.bind(this));
      this.socket.removeEventListener("error", this.handleError.bind(this));
    }

    this.socket = new WebSocket(this.url, this.protocols);
    this.socket.addEventListener("open", this.handleOpen.bind(this));
    this.socket.addEventListener("message", this.handleMessage.bind(this));
    this.socket.addEventListener("close", this.handleClose.bind(this));
    this.socket.addEventListener("error", this.handleError.bind(this));
  }

  private handleOpen(evt: Event) {
    console.log("Opened socket connection", this.url);
    this.emit("open", evt);
    if (this.reconnectTimeout !== undefined) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = undefined;
    }
  }

  private handleMessage(evt: MessageEvent<any>) {
    this.emit("message", evt);
  }

  private handleClose(evt: CloseEvent) {
    this.emit("close", evt);
    this.handleDisconnect(evt);
  }

  private handleError(evt: Event) {
    this.emit("error", evt);
    this.handleDisconnect(evt);
  }

  private handleDisconnect(evt: Event) {
    console.log(
      "Socket disconnected, will attempt to reconnect",
      this.url,
      evt,
    );
    this.reconnectTimeout = setTimeout(this.reinitialize.bind(this), 2500);
  }
}

export default PersistentSocket;
