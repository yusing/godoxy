let ready = false;

window.onload = async function () {
  const consoleEl = document.getElementById("console");
  const loadingDotsEl = document.getElementById("loading-dots");

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

  function addConsoleLine(type, message, timestamp) {
    const line = document.createElement("div");
    line.className = `console-line ${type}`;

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

  // Connect to SSE endpoint
  const eventSource = new EventSource(wakeEventsPath);

  eventSource.onmessage = function (event) {
    const data = JSON.parse(event.data);

    if (data.type === "ready") {
      ready = true;
      // Container is ready, hide loading dots and refresh
      loadingDotsEl.style.display = "none";
      addConsoleLine(
        data.type,
        "Container is ready, refreshing...",
        data.timestamp
      );
      setTimeout(() => {
        window.location.reload();
      }, 200);
    } else if (data.type === "error") {
      // Show error message and hide loading dots
      const errorMessage = data.error || data.message;
      addConsoleLine(data.type, errorMessage, data.timestamp);
      loadingDotsEl.style.display = "none";
      eventSource.close();
    } else {
      // Show other message types
      addConsoleLine(data.type, data.message, data.timestamp);
    }
  };

  eventSource.onerror = function (event) {
    if (ready) {
      // event will be closed by the server
      return;
    }
    addConsoleLine(
      "error",
      "Connection lost. Please refresh the page.",
      new Date().toISOString()
    );
    loadingDotsEl.style.display = "none";
    eventSource.close();
  };
};
