import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import process from "node:process";

import {
  EnvelopeSchema,
  ErrorResponseSchema,
  MsgId as CommonMsgId,
} from "./gen/gsr/common/v1/common_pb.js";
import {
  EchoRequestSchema,
  EchoResponseSchema,
  MsgId as EchoMsgId,
} from "./gen/gsr/example/echo/v1/echo_pb.js";
import {
  authenticatedFrame,
  completeClientHandshake,
  createClientHandshake,
} from "./secure_auth.js";

interface ClientOptions {
  url: string;
  text: string;
  timeoutMs: number;
  secure: boolean;
}

const defaultURL = "ws://127.0.0.1:7888/ws";
const requestID = 1;
const responseTimeoutMs = 5_000;

function parseOptions(arguments_: string[]): ClientOptions {
  const options: ClientOptions = {
    url: defaultURL,
    text: "hello from TypeScript",
    timeoutMs: responseTimeoutMs,
    secure: false,
  };
  for (let index = 0; index < arguments_.length; index += 1) {
    const argument = arguments_[index];
    const value = arguments_[index + 1];
    if (argument === "--url" && value) {
      options.url = value;
      index += 1;
    } else if (argument === "--text" && value !== undefined) {
      options.text = value;
      index += 1;
    } else if (argument === "--timeout-ms" && value) {
      const timeoutMs = Number(value);
      if (!Number.isSafeInteger(timeoutMs) || timeoutMs <= 0) {
        throw new Error("--timeout-ms must be a positive integer");
      }
      options.timeoutMs = timeoutMs;
      index += 1;
    } else if (argument === "--secure") {
      options.secure = true;
    } else {
      throw new Error(`unknown or incomplete argument: ${argument}`);
    }
  }
  return options;
}

function waitForOpen(connection: WebSocket, timeoutMs: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => finish(new Error("WebSocket open timed out")), timeoutMs);
    const finish = (error?: Error) => {
      clearTimeout(timeout);
      connection.removeEventListener("open", handleOpen);
      connection.removeEventListener("error", handleError);
      connection.removeEventListener("close", handleClose);
      if (error) {
        reject(error);
      } else {
        resolve();
      }
    };
    const handleOpen = () => finish();
    const handleError = () => finish(new Error("WebSocket connection failed"));
    const handleClose = (event: Event) => finish(new Error(`WebSocket closed before opening: ${event.type}`));
    connection.addEventListener("open", handleOpen, { once: true });
    connection.addEventListener("error", handleError, { once: true });
    connection.addEventListener("close", handleClose, { once: true });
  });
}

function waitForMessage(connection: WebSocket, timeoutMs: number): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => finish(new Error("response timed out")), timeoutMs);
    const finish = (error?: Error, frame?: Uint8Array) => {
      clearTimeout(timeout);
      connection.removeEventListener("message", handleMessage);
      connection.removeEventListener("error", handleError);
      connection.removeEventListener("close", handleClose);
      if (error) {
        reject(error);
      } else if (frame) {
        resolve(frame);
      } else {
        reject(new Error("WebSocket returned an empty response"));
      }
    };
    const handleMessage = async (event: MessageEvent<unknown>) => {
      try {
        if (event.data instanceof ArrayBuffer) {
          finish(undefined, new Uint8Array(event.data));
        } else if (event.data instanceof Blob) {
          finish(undefined, new Uint8Array(await event.data.arrayBuffer()));
        } else {
          finish(new Error("server response is not a binary WebSocket message"));
        }
      } catch (error) {
        finish(error instanceof Error ? error : new Error(String(error)));
      }
    };
    const handleError = () => finish(new Error("WebSocket failed while waiting for a response"));
    const handleClose = () => finish(new Error("WebSocket closed before returning a response"));
    connection.addEventListener("message", handleMessage, { once: true });
    connection.addEventListener("error", handleError, { once: true });
    connection.addEventListener("close", handleClose, { once: true });
  });
}

async function run(options: ClientOptions): Promise<void> {
  const connection = new WebSocket(options.url);
  connection.binaryType = "arraybuffer";
  try {
    await waitForOpen(connection, options.timeoutMs);

    const payload = toBinary(EchoRequestSchema, create(EchoRequestSchema, { text: options.text }));
    const request = create(EnvelopeSchema, {
      msgId: EchoMsgId.C2S_ECHO_REQUEST,
      requestId: requestID,
      payload,
    });
    let response: Promise<Uint8Array>;
    if (options.secure) {
      const handshake = createClientHandshake();
      const handshakeResponse = waitForMessage(connection, options.timeoutMs);
      connection.send(handshake.frame);
      const authenticator = completeClientHandshake(handshake, await handshakeResponse);
      response = waitForMessage(connection, options.timeoutMs);
      connection.send(authenticatedFrame(authenticator, toBinary(EnvelopeSchema, request)));
    } else {
      response = waitForMessage(connection, options.timeoutMs);
      connection.send(toBinary(EnvelopeSchema, request));
    }

    const responseEnvelope = fromBinary(EnvelopeSchema, await response);
    if (responseEnvelope.requestId !== requestID) {
      throw new Error(`response request_id ${responseEnvelope.requestId} does not match ${requestID}`);
    }
    if (responseEnvelope.msgId === CommonMsgId.S2C_ERROR_RESPONSE) {
      const error = fromBinary(ErrorResponseSchema, responseEnvelope.payload);
      throw new Error(`server error ${error.code}: ${error.message}`);
    }
    if (responseEnvelope.msgId !== EchoMsgId.S2C_ECHO_RESPONSE) {
      throw new Error(`unexpected response msg_id ${responseEnvelope.msgId}`);
    }

    const result = fromBinary(EchoResponseSchema, responseEnvelope.payload);
    console.log(`request_id=${responseEnvelope.requestId} echo=${result.text}`);
  } finally {
    if (connection.readyState === WebSocket.OPEN) {
      connection.close(1000, "client done");
    } else if (connection.readyState === WebSocket.CONNECTING) {
      connection.close();
    }
  }
}

try {
  await run(parseOptions(process.argv.slice(2)));
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
