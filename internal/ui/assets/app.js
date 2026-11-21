(() => {
  let ws = null;
  let state = null;
  let targets = [];
  let audioLevel = 0;
  let waveformHistory = new Array(28).fill(0.06);
  let activeTab = "chat";
  let webSpeech = null;
  let webSpeechActive = false;

  const $ = (id) => document.getElementById(id);

  function fmtTime(sec) {
    const s = Math.max(0, Math.floor(sec || 0));
    const m = Math.floor(s / 60);
    const r = s % 60;
    return `${String(m).padStart(2, "0")}:${String(r).padStart(2, "0")}`;
  }

  function send(cmd) {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(cmd));
    }
  }

  function connectWS() {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    ws = new WebSocket(`${proto}//${location.host}/ws`);

    ws.onopen = () => {
      refreshTargets();
    };

    ws.onmessage = (evt) => {
      try {
        const msg = JSON.parse(evt.data);
        if (msg.type === "state") {
          state = msg.payload;
          renderState();
        } else if (msg.type === "audio_level") {
          audioLevel = msg.payload.level || 0;
        } else if (msg.type === "live_frame") {
          const img = $("live-preview-img");
          img.src = msg.payload.data_url;
          img.classList.remove("hidden");
          $("preview-placeholder").classList.add("hidden");
        }
      } catch (e) {
        console.error("WS parse error", e);
      }
    };

    ws.onclose = () => {
      setTimeout(connectWS, 1200);
    };
  }

  async function refreshTargets() {
    try {
      const res = await fetch("/api/targets");
      targets = await res.json();
      const sel = $("target-select");
      const prev = sel.value;
      sel.innerHTML = "";
      targets.forEach((t) => {
        const opt = document.createElement("option");
        opt.value = t.id;
        const icon = t.type === "display" ? "🖥" : "🪟";
        opt.textContent = `${icon} ${t.title} (${t.resolution.width}×${t.resolution.height})`;
        sel.appendChild(opt);
      });
      if (prev && targets.some((t) => t.id === prev)) {
        sel.value = prev;
      }
      updateTargetBadge();
    } catch (e) {
      console.error("Failed to fetch targets", e);
    }
  }

  function updateTargetBadge() {
    const sel = $("target-select");
    const found = targets.find((t) => t.id === sel.value);
    $("preview-target-pill").textContent = found ? found.title : "No target selected";
  }

  function renderState() {
    if (!state) return;

    // Switch Studio vs Compact Floating HUD
    if (state.hud_compact_mode) {
      $("studio-shell").classList.add("hidden");
      $("hud-shell").classList.remove("hidden");
    } else {
      $("hud-shell").classList.add("hidden");
      $("studio-shell").classList.remove("hidden");
    }

    // Populate default workspace once
    if (!$("workspace-input").value && state.workspace_path) {
      $("workspace-input").value = state.workspace_path;
    }

    // Timers & REC badges
    const timeStr = fmtTime(state.elapsed_seconds);
    $("top-timer").textContent = timeStr;
    $("hud-timer").textContent = timeStr;

    const topBadge = $("top-state-badge");
    const prevRec = $("preview-rec-pill");
    const hudDot = $("hud-rec-dot");
    const hudLabel = $("hud-rec-label");

    if (state.recording) {
      topBadge.textContent = state.paused ? "PAUSED" : "REC LIVE";
      topBadge.className = state.paused
        ? "xnk-badge xnk-badge-amber xnk-badge-pill"
        : "xnk-badge xnk-badge-crimson xnk-badge-pill";
      prevRec.textContent = state.paused ? "⏸ PAUSED" : "● REC";
      prevRec.className = state.paused
        ? "xnk-badge xnk-badge-amber xnk-badge-pill"
        : "xnk-badge xnk-badge-crimson xnk-badge-pill";
      prevRec.classList.remove("hidden");

      hudDot.classList.toggle("is-live", !state.paused);
      hudLabel.textContent = state.paused ? "PAUSED" : "REC";

      $("btn-start").classList.add("hidden");
      $("btn-stop").classList.remove("hidden");
      $("btn-pause").disabled = false;
      $("btn-pause").innerHTML = state.paused
        ? '▶ Resume <span class="xnk-kbd">Space</span>'
        : '⏸ Pause <span class="xnk-kbd">Space</span>';
      $("hud-pause-btn").textContent = state.paused ? "▶" : "⏸";
    } else {
      topBadge.textContent = "STANDBY";
      topBadge.className = "xnk-badge xnk-badge-pill";
      prevRec.classList.add("hidden");
      hudDot.classList.remove("is-live");
      hudLabel.textContent = "STANDBY";

      $("btn-start").classList.remove("hidden");
      $("btn-stop").classList.add("hidden");
      $("btn-pause").disabled = true;
    }

    // Antigravity status
    const agyBadge = $("top-agy-badge");
    agyBadge.textContent = (state.agent_state || "idle").toUpperCase();
    $("agent-status-sub").textContent = state.agent_status_text || "Ready";
    $("hud-agy-status").textContent = `AGY: ${state.agent_state || "ready"}`;
    $("top-platform").textContent = state.permissions?.platform || "—";

    // Thinking indicator
    const isThinking = state.agent_state === "thinking" || state.agent_state === "working";
    $("thinking-indicator").classList.toggle("hidden", !isThinking);

    // Voice mute button
    $("btn-mute-voice").textContent = state.voice_output_muted ? "🔇 Voice Muted" : "🔊 Voice On";

    // Batch mode indicators
    const batchBadge = $("batch-status-badge");
    const hudBatchPill = $("hud-batch-pill");
    const flushBtn = $("btn-flush-batch");
    if (state.batch_mode) {
      batchBadge.textContent = `BATCH HOLD (${state.batched_count})`;
      batchBadge.className = "xnk-badge xnk-badge-amber xnk-badge-pill";
      hudBatchPill.textContent = `BATCH ${state.batched_count}`;
      hudBatchPill.classList.remove("hidden");
      flushBtn.classList.toggle("hidden", state.batched_count === 0);
      flushBtn.textContent = `Send Batch (${state.batched_count})`;
    } else {
      batchBadge.textContent = "AUTO-STREAM";
      batchBadge.className = "xnk-badge xnk-badge-live xnk-badge-pill";
      hudBatchPill.classList.add("hidden");
      flushBtn.classList.add("hidden");
    }

    // Live Transcript Ribbon
    const liveText = state.live_transcript || "Listening for director commentary…";
    $("live-transcript-box").textContent = liveText;
    $("hud-transcript").textContent = liveText;

    // Permission Pills
    renderPermissions(state.permissions);

    // Pending Voice Approval Banner
    const perm = state.pending_approval;
    if (perm) {
      const desc = perm.command
        ? `${perm.tool_name}: ${perm.command}`
        : `${perm.tool_name}: ${perm.description || "Confirm action"}`;
      $("studio-approval-desc").textContent = desc;
      $("studio-approval-box").classList.remove("hidden");
      $("hud-approval-text").textContent = desc;
      $("hud-approval-strip").classList.remove("hidden");
    } else {
      $("studio-approval-box").classList.add("hidden");
      $("hud-approval-strip").classList.add("hidden");
    }

    // Last exported bundle banner
    if (state.last_bundle_path && !state.recording) {
      $("last-bundle-text").textContent = `Archive sealed: ${state.last_bundle_path}`;
      $("last-bundle-banner").classList.remove("hidden");
    } else {
      $("last-bundle-banner").classList.add("hidden");
    }

    // Chat stream & Utterances timeline
    renderChat(state.chat_history || []);
    renderUtterances(state.utterances || []);
    $("utt-count").textContent = String((state.utterances || []).length);
  }

  function renderPermissions(p) {
    if (!p) return;
    const container = $("perm-pills");
    const items = [
      { label: "AGY", ok: p.agy_available },
      { label: "FFMPEG", ok: p.ffmpeg_available },
      { label: "SCR", ok: p.screen_recording === "granted" },
      { label: "AX", ok: p.accessibility === "granted" },
    ];
    container.innerHTML = items
      .map(
        (it) =>
          `<span class="xnk-badge ${it.ok ? "xnk-badge-live" : "xnk-badge-amber"}">${it.label}</span>`
      )
      .join("");
  }

  function escapeHtml(s) {
    return String(s || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }

  function renderChat(entries) {
    const stream = $("chat-stream");
    if (!entries.length) {
      stream.innerHTML = `
        <div class="xnk-empty-state">
          <strong>No live turns yet.</strong><br />
          Start a Director Session and speak naturally — or type below to inject a director comment with Trigger &amp; Reaction frames.
        </div>`;
      return;
    }

    stream.innerHTML = entries
      .map((e) => {
        if (e.role === "system") {
          return `<div class="sys-note">${escapeHtml(e.text)}</div>`;
        }
        if (e.role === "tool") {
          return `<div class="tool-chip">⚙ ${escapeHtml(e.text)}</div>`;
        }
        const cls = e.role === "user" ? "is-user" : "is-ai";
        const roleLabel =
          e.role === "user"
            ? "Director (Voice)"
            : `<span class="xnk-orb-avatar"></span> Antigravity`;
        const ts = e.timestamp ? new Date(e.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "";
        let thumbs = "";
        if (e.frame_paths && e.frame_paths.length) {
          thumbs =
            `<div class="frame-strip">` +
            e.frame_paths
              .map(
                (fp, idx) =>
                  `<img class="frame-thumb" src="/api/frame?path=${encodeURIComponent(fp)}" data-frame-path="${escapeHtml(fp)}" data-frame-label="${idx === 0 ? "Trigger Frame" : "Reaction Frame"}" title="${idx === 0 ? "Trigger Frame" : "Reaction Frame"}" />`
              )
              .join("") +
            `</div>`;
        }
        return `
          <div class="xnk-chat-msg ${cls}">
            <div class="xnk-chat-msg-header">
              <span class="xnk-chat-msg-role">${roleLabel}</span>
              <span class="xnk-chat-msg-time">${ts}</span>
            </div>
            <div class="xnk-prose">${escapeHtml(e.text).replace(/\n/g, "<br/>")}</div>
            ${thumbs}
          </div>`;
      })
      .join("");

    const scroller = $("panel-chat");
    scroller.scrollTop = scroller.scrollHeight;
  }

  function renderUtterances(utts) {
    const list = $("utterance-list");
    if (!utts.length) {
      list.innerHTML = `<div class="xnk-empty-state">No director utterances recorded yet.</div>`;
      return;
    }
    list.innerHTML = utts
      .slice()
      .reverse()
      .map((u) => {
        const lb = u.lookback_window || {};
        const frames = [u.trigger_frame_path, u.reaction_frame_path].filter(Boolean);
        const thumbs = frames.length
          ? `<div class="frame-strip">` +
            frames
              .map(
                (fp, idx) =>
                  `<img class="frame-thumb" src="/api/frame?path=${encodeURIComponent(fp)}" data-frame-path="${escapeHtml(fp)}" data-frame-label="Utterance #${u.id} ${idx === 0 ? "Trigger" : "Reaction"}" />`
              )
              .join("") +
            `</div>`
          : "";
        return `
          <div class="utt-card ${u.discarded ? "is-discarded" : ""}">
            <div class="utt-meta">
              <span>#${u.id} · Speech ${fmtTime(u.speech_start)}–${fmtTime(u.speech_end)}</span>
              <span>Lookback ${(lb.start_seconds || 0).toFixed(1)}s–${(lb.end_seconds || 0).toFixed(1)}s</span>
            </div>
            <div class="utt-text">${escapeHtml(u.transcript)}</div>
            ${thumbs}
          </div>`;
      })
      .join("");
  }

  // Animated 60fps Audio Waveform Visualizer
  function drawWaveforms() {
    waveformHistory.shift();
    const targetVal = state && state.recording && !state.paused
      ? Math.max(0.08, Math.min(1.0, audioLevel + Math.random() * 0.04))
      : 0.06;
    waveformHistory.push(targetVal);

    drawCanvas($("studio-waveform"), waveformHistory, "#38bdf8");
    drawCanvas($("hud-waveform"), waveformHistory.slice(-14), "#38bdf8");
    requestAnimationFrame(drawWaveforms);
  }

  function drawCanvas(canvas, bars, color) {
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    const w = canvas.width;
    const h = canvas.height;
    ctx.clearRect(0, 0, w, h);

    const gap = 3;
    const bw = Math.max(2, (w - gap * (bars.length - 1)) / bars.length);
    ctx.fillStyle = color;
    bars.forEach((v, i) => {
      const bh = Math.max(3, v * (h - 4));
      const x = i * (bw + gap);
      const y = (h - bh) / 2;
      ctx.beginPath();
      ctx.roundRect(x, y, bw, bh, 2);
      ctx.fill();
    });
  }

  // Optional Browser Web Speech API Fallback (great for Windows/Linux or testing without macOS speech perms)
  function toggleWebSpeech() {
    const SR = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SR) {
      alert("Web Speech API is not supported in this WebView; native OS speech engine is used instead.");
      return;
    }
    if (webSpeechActive && webSpeech) {
      webSpeechActive = false;
      webSpeech.stop();
      $("btn-web-mic").textContent = "🎙 Browser STT";
      return;
    }
    webSpeech = new SR();
    webSpeech.continuous = true;
    webSpeech.interimResults = true;
    webSpeech.lang = "en-US";
    webSpeech.onresult = (e) => {
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const text = e.results[i][0].transcript;
        if (e.results[i].isFinal) {
          send({ action: "inject_utterance", text });
        } else {
          $("live-transcript-box").textContent = text;
          $("hud-transcript").textContent = text;
        }
      }
    };
    webSpeech.onend = () => {
      if (webSpeechActive) webSpeech.start();
    };
    webSpeechActive = true;
    webSpeech.start();
    $("btn-web-mic").textContent = "🔴 Browser STT On";
  }

  // Wire UI Buttons & Hotkeys
  function bindEvents() {
    $("target-select").addEventListener("change", updateTargetBadge);
    $("btn-refresh-targets").addEventListener("click", refreshTargets);

    $("btn-start").addEventListener("click", () => {
      send({
        action: "start_session",
        target_id: $("target-select").value,
        workspace_path: $("workspace-input").value.trim(),
        conversation_id: $("conv-id-input").value.trim(),
        continue_session: $("continue-checkbox").checked,
      });
    });

    $("btn-stop").addEventListener("click", () => send({ action: "stop_session" }));
    $("hud-stop-btn").addEventListener("click", () => send({ action: "stop_session" }));

    $("btn-pause").addEventListener("click", () => send({ action: "toggle_pause" }));
    $("hud-pause-btn").addEventListener("click", () => send({ action: "toggle_pause" }));

    $("btn-batch").addEventListener("click", () => send({ action: "toggle_batch" }));
    $("hud-batch-btn").addEventListener("click", () => send({ action: "toggle_batch" }));
    $("btn-flush-batch").addEventListener("click", () => send({ action: "flush_batch" }));
    $("btn-discard").addEventListener("click", () => send({ action: "discard_last" }));

    $("btn-mute-voice").addEventListener("click", () => send({ action: "toggle_mute" }));
    $("btn-compact-hud").addEventListener("click", () => send({ action: "toggle_hud_mode" }));
    $("hud-expand-btn").addEventListener("click", () => send({ action: "toggle_hud_mode" }));
    $("btn-web-mic").addEventListener("click", toggleWebSpeech);

    // Approvals
    const approve = () => send({ action: "resolve_approval", request_id: state?.pending_approval?.id || "", decision: "approve" });
    const deny = () => send({ action: "resolve_approval", request_id: state?.pending_approval?.id || "", decision: "deny" });
    $("studio-approve-btn").addEventListener("click", approve);
    $("studio-deny-btn").addEventListener("click", deny);
    $("hud-approve-btn").addEventListener("click", approve);
    $("hud-deny-btn").addEventListener("click", deny);

    // Tabs
    $("tab-chat").addEventListener("click", () => {
      activeTab = "chat";
      $("tab-chat").classList.add("is-active");
      $("tab-timeline").classList.remove("is-active");
      $("panel-chat").classList.remove("hidden");
      $("panel-timeline").classList.add("hidden");
    });
    $("tab-timeline").addEventListener("click", () => {
      activeTab = "timeline";
      $("tab-timeline").classList.add("is-active");
      $("tab-chat").classList.remove("is-active");
      $("panel-timeline").classList.remove("hidden");
      $("panel-chat").classList.add("hidden");
    });

    // Manual Director Turn
    $("manual-form").addEventListener("submit", (e) => {
      e.preventDefault();
      const input = $("manual-input");
      const text = input.value.trim();
      if (!text) return;
      send({ action: "inject_utterance", text });
      input.value = "";
    });

    // Frame Lightbox Click Delegation
    document.addEventListener("click", (e) => {
      const thumb = e.target.closest(".frame-thumb");
      if (thumb) {
        $("frame-modal-img").src = thumb.src;
        $("frame-modal-title").textContent = thumb.dataset.frameLabel || "Frame Snapshot";
        $("frame-modal").classList.remove("hidden");
      }
    });
    $("frame-modal-close").addEventListener("click", () => $("frame-modal").classList.add("hidden"));
    $("frame-modal").addEventListener("click", (e) => {
      if (e.target === $("frame-modal")) $("frame-modal").classList.add("hidden");
    });

    // Global Keyboard Shortcuts inside App
    document.addEventListener("keydown", (e) => {
      const inInput = document.activeElement && ["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement.tagName);
      if (e.metaKey && e.key.toLowerCase() === "y" && state?.pending_approval) {
        e.preventDefault();
        approve();
      } else if (e.metaKey && e.key.toLowerCase() === "n" && state?.pending_approval) {
        e.preventDefault();
        deny();
      } else if (e.altKey && e.key.toLowerCase() === "b") {
        e.preventDefault();
        send({ action: "toggle_batch" });
      } else if (!inInput && e.code === "Space" && state?.recording) {
        e.preventDefault();
        send({ action: "toggle_pause" });
      } else if (!inInput && e.key === "Escape") {
        if (!$("frame-modal").classList.contains("hidden")) {
          $("frame-modal").classList.add("hidden");
        } else if (state?.recording) {
          send({ action: "stop_session" });
        }
      }
    });
  }

  bindEvents();
  connectWS();
  drawWaveforms();
})();
