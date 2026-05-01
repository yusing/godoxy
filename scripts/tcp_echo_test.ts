#!/usr/bin/env bun
import { spawn } from "node:child_process";
import net from "node:net";
import tls from "node:tls";

const proxyHost = "127.0.0.1";
const proxyPort = 443;
const composeFile = "compose.yml";
const echoService = "tcp_echo_server";
const routeReadyTimeoutMs = 30_000;

type EchoTest = {
  name: string;
  serverName: string;
  payload: Buffer;
};

type CommandResult = {
  code: number;
  output: string;
};

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

function runCommand(command: string, args: string[]): Promise<CommandResult> {
  return new Promise((resolve) => {
    const child = spawn(command, args, { stdio: ["ignore", "pipe", "pipe"] });
    let output = "";

    child.stdout.on("data", (chunk) => {
      output += chunk.toString();
    });
    child.stderr.on("data", (chunk) => {
      output += chunk.toString();
    });
    child.once("close", (code) => {
      resolve({ code: code ?? 1, output });
    });
  });
}

async function ensureEchoServiceRunning(): Promise<void> {
  const result = await runCommand("docker", [
    "compose",
    "-f",
    composeFile,
    "up",
    "-d",
    echoService,
  ]);
  if (result.code !== 0) {
    throw new Error(`failed to start ${echoService}: ${result.output.trim()}`);
  }
  if (result.output.trim().length > 0) {
    console.log(result.output.trim());
  }
}

async function waitPort(port: number): Promise<void> {
  const deadline = Date.now() + 10_000;
  let lastError: unknown;

  while (Date.now() < deadline) {
    try {
      const socket = await connectPlain(port);
      socket.destroy();
      return;
    } catch (error) {
      lastError = error;
      await delay(100);
    }
  }

  throw new Error(`GoDoxy localhost:${port} not ready: ${String(lastError)}`);
}

function connectPlain(port: number): Promise<net.Socket> {
  return new Promise((resolve, reject) => {
    const socket = net.createConnection({ host: proxyHost, port });
    const timeout = setTimeout(() => {
      socket.destroy();
      reject(new Error(`connect timeout on ${proxyHost}:${port}`));
    }, 500);

    socket.once("connect", () => {
      clearTimeout(timeout);
      resolve(socket);
    });
    socket.once("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
  });
}

function connectTLS(serverName: string): Promise<tls.TLSSocket> {
  return new Promise((resolve, reject) => {
    const socket = tls.connect({
      host: proxyHost,
      port: proxyPort,
      servername: serverName,
      rejectUnauthorized: false,
    });
    const timeout = setTimeout(() => {
      socket.destroy();
      reject(new Error(`tls connect timeout for SNI ${serverName}`));
    }, 5_000);

    socket.once("secureConnect", () => {
      clearTimeout(timeout);
      resolve(socket);
    });
    socket.once("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
  });
}

function readExact(socket: net.Socket, expectedBytes: number): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    let receivedBytes = 0;

    const cleanup = () => {
      socket.off("data", onData);
      socket.off("error", onError);
      socket.off("end", onEnd);
      socket.off("timeout", onTimeout);
    };

    const onData = (chunk: Buffer) => {
      chunks.push(chunk);
      receivedBytes += chunk.length;
      if (receivedBytes >= expectedBytes) {
        cleanup();
        resolve(
          Buffer.concat(chunks, receivedBytes).subarray(0, expectedBytes),
        );
      }
    };

    const onError = (error: Error) => {
      cleanup();
      reject(error);
    };

    const onEnd = () => {
      cleanup();
      resolve(Buffer.concat(chunks, receivedBytes));
    };

    const onTimeout = () => {
      cleanup();
      socket.destroy();
      reject(
        new Error(`read timeout after ${receivedBytes}/${expectedBytes} bytes`),
      );
    };

    socket.setTimeout(5_000);
    socket.on("data", onData);
    socket.once("error", onError);
    socket.once("end", onEnd);
    socket.once("timeout", onTimeout);
  });
}

async function expectEcho(test: EchoTest): Promise<void> {
  const socket = await connectTLS(test.serverName);
  socket.write(test.payload);
  const received = await readExact(socket, test.payload.length);
  socket.destroy();

  if (!received.equals(test.payload)) {
    throw new Error(
      `echo mismatch: got ${JSON.stringify(received.toString())}`,
    );
  }
}

async function waitForRoute(test: EchoTest): Promise<void> {
  const deadline = Date.now() + routeReadyTimeoutMs;
  let lastError: unknown;

  while (Date.now() < deadline) {
    try {
      await expectEcho(test);
      return;
    } catch (error) {
      lastError = error;
      await delay(500);
    }
  }

  throw new Error(`route ${test.serverName} not ready: ${String(lastError)}`);
}

async function main(): Promise<number> {
  const tests: EchoTest[] = [
    {
      name: "godoxy TLS passthrough",
      serverName: "tcp-echo-passthrough.my.app",
      payload: Buffer.from("godoxy tls passthrough echo"),
    },
    {
      name: "godoxy TLS termination",
      serverName: "tcp-echo-termination.my.app",
      payload: Buffer.from("godoxy tls termination echo"),
    },
  ];

  try {
    await ensureEchoServiceRunning();
    await waitPort(proxyPort);
  } catch (error) {
    console.log(`tcp-echo-test: FAIL: ${String(error)}`);
    return 1;
  }

  let failed = false;
  for (const test of tests) {
    try {
      await waitForRoute(test);
      console.log(`${test.name}: PASS`);
    } catch (error) {
      failed = true;
      console.log(`${test.name}: FAIL: ${String(error)}`);
    }
  }

  if (failed) return 1;

  console.log("tcp-echo-test: PASS");
  return 0;
}

process.exit(await main());
