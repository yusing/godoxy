/**
 * @typedef {"debug"|"info"|"warn"|"error"} EventLevel
 * @see goutils/events/level.go
 */

/**
 * @typedef {{ message?: string, error?: string }} WakeEvent
 * @see internal/idlewatcher/events.go WakeEvent
 */

/**
 * @typedef {"starting"|"waking_dep"|"dep_ready"|"container_woke"|"waiting_ready"|"ready"|"error"} WakeEventType
 * @see internal/idlewatcher/events.go WakeEventType
 */

/**
 * @typedef {{ timestamp: string, level: EventLevel, category: string, action: WakeEventType, data: WakeEvent }} WakeSSEEvent
 * @see goutils/events/event.go Event
 */

let ready = false;

window.onload = async () => {
  const consoleEl = document.getElementById("console");
  const loadingDotsEl = document.getElementById("loading-dots");

  if (!consoleEl || !loadingDotsEl) {
    console.error("Required DOM elements not found");
    return;
  }

  /**
   * @param {string} timestamp - ISO timestamp string
   * @returns {string}
   */
  function formatTimestamp(timestamp) {
    const date = new Date(timestamp);
    return date.toLocaleTimeString("en-US", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      fractionalSecondDigits: 3,
    });
  }

  /**
   * @param {string} type - Console line type (e.g. ready, error, or WakeEventType)
   * @param {string} message
   * @param {string} timestamp - ISO timestamp string
   * @param {EventLevel} [level]
   */
  function addConsoleLine(type, message, timestamp, level) {
    const validLevels = ["debug", "info", "warn", "error"];
    const lvl = validLevels.includes(level)
      ? level
      : type === "error"
        ? "error"
        : "info";
    const line = document.createElement("div");
    line.className = `console-line ${type} level-${lvl}`;

    const timestampEl = document.createElement("span");
    timestampEl.className = "console-timestamp";
    timestampEl.textContent = formatTimestamp(timestamp);

    const messageEl = document.createElement("span");
    messageEl.className = "console-message";
    messageEl.textContent = message;

    line.appendChild(timestampEl);
    line.appendChild(messageEl);

    consoleEl.appendChild(line);
    consoleEl.scrollTop = consoleEl.scrollHeight;
  }

  if (typeof wakeEventsPath === "undefined" || !wakeEventsPath) {
    addConsoleLine(
      "error",
      "Configuration error: wakeEventsPath not defined",
      new Date().toISOString(),
    );
    loadingDotsEl.style.display = "none";
    return;
  }

  if (typeof EventSource === "undefined") {
    addConsoleLine(
      "error",
      "Browser does not support Server-Sent Events",
      new Date().toISOString(),
    );
    loadingDotsEl.style.display = "none";
    return;
  }

  // Connect to SSE endpoint
  const eventSource = new EventSource(wakeEventsPath);

  eventSource.onmessage = (event) => {
    /** @type {WakeSSEEvent} */
    let evt;
    try {
      evt = JSON.parse(event.data);
    } catch {
      addConsoleLine(
        "error",
        `Invalid event data: ${event.data}`,
        new Date().toISOString(),
      );
      return;
    }

    const payload = evt.data || {};
    const type = evt.action;
    const timestamp = evt.timestamp;

    if (type === "ready") {
      ready = true;
      // Container is ready, hide loading dots and refresh
      loadingDotsEl.style.display = "none";
      addConsoleLine(
        type,
        "Container is ready, refreshing...",
        timestamp,
        evt.level,
      );
      setTimeout(() => {
        window.location.reload();
      }, 200);
    } else if (type === "error" || evt.level === "error") {
      // Show error message and hide loading dots
      const errorMessage = payload.error || payload.message || "Unknown error";
      addConsoleLine(type, errorMessage, timestamp, evt.level);
      loadingDotsEl.style.display = "none";
      eventSource.close();
    } else {
      // Show other message types
      const message = payload.message || "";
      addConsoleLine(type, message, timestamp, evt.level);
    }
  };

  eventSource.onerror = () => {
    if (ready) {
      // event will be closed by the server
      return;
    }
    addConsoleLine(
      "error",
      "Connection lost. Please refresh the page.",
      new Date().toISOString(),
    );
    loadingDotsEl.style.display = "none";
    eventSource.close();
  };
};
