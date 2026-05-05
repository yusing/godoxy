#!/usr/bin/env bun
import { spawn } from "node:child_process";
import { copyFile, mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import os from "node:os";

const URL = "https://idlewatcher-test-microbin.my.app";
const OUTPUT_PATH = path.join("screenshots", "idlesleeper.webp");
const GALLERY_COPY_PATH = path.join(
  "webui",
  "wiki",
  "content",
  "docs",
  "godoxy",
  "images",
  "gallery",
  "idlesleeper.webp",
);
const COMPOSE_FILE = "dev.compose.yml";
const IDLEWATCHER_CONTAINER = "idlewatcher-test-microbin";
const WIDTH = 2560;
const HEIGHT = 1440;
const ZOOM_PERCENT = 125;
const RECORD_MS = 2000;
const CAPTURE_INTERVAL_MS = 50;
const PLAYBACK_FPS = 15;
const CHROME_PORT = 9226;

type CdpResponse<T> =
  | {
      id: number;
      result: T;
    }
  | {
      id: number;
      error: {
        code: number;
        message: string;
      };
    };

type JsonVersionResponse = {
  webSocketDebuggerUrl: string;
};

const workspaceRoot = await mkdtemp(path.join(os.tmpdir(), "idlesleeper-"));

process.on("SIGINT", () => {
  void cleanupAndExit(130);
});
process.on("SIGTERM", () => {
  void cleanupAndExit(143);
});

const chrome = spawn(
  "chromium",
  [
    "--headless=new",
    "--disable-gpu",
    "--ignore-certificate-errors",
    "--allow-insecure-localhost",
    `--remote-debugging-port=${CHROME_PORT}`,
    `--window-size=${WIDTH},${HEIGHT}`,
    "--hide-scrollbars",
    "about:blank",
  ],
  { stdio: ["ignore", "pipe", "pipe"] },
);

chrome.stdout.on("data", (chunk) => {
  process.stderr.write(chunk);
});
chrome.stderr.on("data", (chunk) => {
  process.stderr.write(chunk);
});

try {
  await runRecording();
} finally {
  chrome.kill("SIGTERM");
  await rm(workspaceRoot, { recursive: true, force: true }).catch(() => {});
}

async function runRecording(): Promise<void> {
  await waitForChrome();
  const version = await fetchJson<JsonVersionResponse>(
    `http://127.0.0.1:${CHROME_PORT}/json/version`,
  );
  const socket = new WebSocket(version.webSocketDebuggerUrl);
  await waitForSocketOpen(socket);

  const frameDir = path.join(workspaceRoot, "frames");
  await rm(frameDir, { recursive: true, force: true });
  await mkdir(frameDir, { recursive: true });
  const connection = new CdpConnection(socket);
  try {
    const { targetId } = await connection.send<{ targetId: string }>(
      "Target.createTarget",
      { url: "about:blank" },
      undefined,
    );
    const { sessionId } = await connection.send<{ sessionId: string }>(
      "Target.attachToTarget",
      { targetId, flatten: true },
      undefined,
    );

    await connection.send("Page.enable", {}, sessionId);
    await connection.send("Runtime.enable", {}, sessionId);
    await connection.send(
      "Emulation.setDeviceMetricsOverride",
      {
        width: WIDTH,
        height: HEIGHT,
        deviceScaleFactor: 1,
        mobile: false,
      },
      sessionId,
    );
    await connection.send(
      "Emulation.setEmulatedMedia",
      {
        media: "",
        features: [{ name: "prefers-color-scheme", value: "dark" }],
      },
      sessionId,
    );
    await connection.send(
      "Page.addScriptToEvaluateOnNewDocument",
      {
        source: `document.documentElement.style.zoom = "${ZOOM_PERCENT}%";`,
      },
      sessionId,
    );

    await stopIdlewatcherContainer();
    await connection.send("Page.navigate", { url: URL }, sessionId);

    let frameIndex = 0;
    const startedAt = Date.now();
    while (Date.now() - startedAt < RECORD_MS) {
      try {
        const screenshot = await connection.send<{ data: string }>(
          "Page.captureScreenshot",
          {
            format: "jpeg",
            quality: 92,
            fromSurface: true,
          },
          sessionId,
        );
        const framePath = path.join(frameDir, `frame-${String(frameIndex).padStart(4, "0")}.jpg`);
        await writeFile(framePath, Buffer.from(screenshot.data, "base64"));
        frameIndex += 1;
      } catch (e: any) {
        if (e instanceof Error && e.message.includes("Not attached to an active page")) {
          await sleep(10);
          continue;
        }
      }
      await sleep(CAPTURE_INTERVAL_MS);
    }

    if (frameIndex < 2) {
      throw new Error(`too few frames captured: ${frameIndex}`);
    }

    await encodeWebp(frameDir, frameIndex);
    await ensureDirectory(path.dirname(OUTPUT_PATH));
    await ensureDirectory(path.dirname(GALLERY_COPY_PATH));
    await copyFile(OUTPUT_PATH, GALLERY_COPY_PATH);

    await connection.send("Target.closeTarget", { targetId }, undefined).catch(() => {});
    socket.close();
  } finally {
    connection.close();
  }
}

async function encodeWebp(frameDir: string, frameCount: number): Promise<void> {
  const pattern = path.join(frameDir, "frame-%04d.jpg");
  await runCommand("ffmpeg", [
    "-y",
    "-hide_banner",
    "-loglevel",
    "error",
    "-framerate",
    String(PLAYBACK_FPS),
    "-start_number",
    "0",
    "-i",
    pattern,
    "-c:v",
    "libwebp_anim",
    "-lossless",
    "0",
    "-q:v",
    "82",
    "-loop",
    "0",
    "-an",
    OUTPUT_PATH,
  ]);

  if (frameCount < 2) {
    throw new Error(`webp encode input too small: ${frameCount} frames`);
  }
}

async function runCommand(command: string, args: string[]): Promise<void> {
  const child = spawn(command, args, { stdio: ["ignore", "pipe", "pipe"] });
  let output = "";

  child.stdout.on("data", (chunk) => {
    output += chunk.toString();
  });
  child.stderr.on("data", (chunk) => {
    output += chunk.toString();
  });

  const code = await new Promise<number>((resolve) => {
    child.once("close", (exitCode) => {
      resolve(exitCode ?? 1);
    });
  });

  if (code !== 0) {
    throw new Error(`${command} failed with exit code ${code}: ${output.trim()}`);
  }
}

async function waitForChrome(): Promise<void> {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://127.0.0.1:${CHROME_PORT}/json/version`);
      if (response.ok) return;
    } catch {
      // retry until deadline
    }
    await sleep(200);
  }

  throw new Error("chromium not ready");
}

async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`fetch failed ${response.status} ${response.statusText}`);
  }
  return (await response.json()) as T;
}

function waitForSocketOpen(socket: WebSocket): Promise<void> {
  return new Promise((resolve, reject) => {
    socket.addEventListener("open", () => resolve(), { once: true });
    socket.addEventListener("error", () => reject(new Error("websocket open failed")), {
      once: true,
    });
  });
}

class CdpConnection {
  private readonly socket: WebSocket;
  private nextId = 0;
  private readonly pending = new Map<
    number,
    {
      resolve: (value: unknown) => void;
      reject: (error: Error) => void;
    }
  >();

  constructor(socket: WebSocket) {
    this.socket = socket;
    this.socket.addEventListener("message", (event) => {
      void this.onMessage(String(event.data));
    });
  }

  close(): void {
    this.socket.close();
    for (const { reject } of this.pending.values()) {
      reject(new Error("cdp connection closed"));
    }
    this.pending.clear();
  }

  send<T = unknown>(
    method: string,
    params: Record<string, unknown> = {},
    sessionId?: string,
  ): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const id = ++this.nextId;
      this.pending.set(id, {
        resolve: (value) => resolve(value as T),
        reject,
      });
      this.socket.send(
        JSON.stringify({
          id,
          method,
          params,
          ...(sessionId ? { sessionId } : {}),
        }),
      );
    });
  }

  private async onMessage(raw: string): Promise<void> {
    const message = JSON.parse(raw) as CdpResponse<unknown> & {
      method?: string;
      params?: Record<string, unknown>;
    };
    if ("id" in message) {
      const entry = this.pending.get(message.id);
      if (!entry) return;
      this.pending.delete(message.id);
      if ("error" in message) {
        entry.reject(new Error(message.error.message));
        return;
      }
      entry.resolve(message.result);
    }
  }
}

function ensureDirectory(dirPath: string): Promise<void> {
  return mkdir(dirPath, { recursive: true });
}

function sleep(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function cleanupAndExit(exitCode: number): Promise<never> {
  await rm(workspaceRoot, { recursive: true, force: true }).catch(() => {});
  process.exit(exitCode);
}

async function stopIdlewatcherContainer(): Promise<void> {
  await runCommand("docker", [
    "compose",
    "-f",
    COMPOSE_FILE,
    "stop",
    IDLEWATCHER_CONTAINER,
    "-t",
    "0",
  ]);
}
