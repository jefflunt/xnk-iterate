(() => {
  let ws = null;
  let state = null;
  let audioLevel = 0;
  let waveformHistory = new Array(16).fill(0.08);
  let activeDrawerTab = "turns";
  let webSpeech = null;
  let webSpeechActive = false;
  let recordingHotkeyTarget = null; // "send_turn" | "discard_turn" | null

  const MOD_CMD = 1;
  const MOD_SHIFT = 2;
  const MOD_OPTION = 4;
  const MOD_CONTROL = 8;

  const MODIFIER_CODES = new Set([
    "MetaLeft", "MetaRight", "AltLeft", "AltRight",
    "ControlLeft", "ControlRight", "ShiftLeft", "ShiftRight",
    "CapsLock", "Fn"
  ]);

  function eventModifiersMask(e) {
    let m = 0;
    if (e.metaKey) m |= MOD_CMD;
    if (e.shiftKey) m |= MOD_SHIFT;
    if (e.altKey) m |= MOD_OPTION;
    if (e.ctrlKey) m |= MOD_CONTROL;
    return m;
  }

  function matchesHotkey(e, binding) {
    if (!binding || !binding.key_code) return false;
    return e.code === binding.key_code && eventModifiersMask(e) === (binding.modifiers || 0);
  }

  function stopHotkeyRecording() {
    if (!recordingHotkeyTarget) return;
    recordingHotkeyTarget = null;
    $("hk-send-btn").classList.remove("is-recording");
    $("hk-discard-btn").classList.remove("is-recording");
    send({ action: "set_hotkey_recording", hotkey_recording: false });
    renderState();
  }

  function startHotkeyRecording(target) {
    if (recordingHotkeyTarget === target) {
      stopHotkeyRecording();
      return;
    }
    recordingHotkeyTarget = target;
    $("hk-send-btn").classList.toggle("is-recording", target === "send_turn");
    $("hk-discard-btn").classList.toggle("is-recording", target === "discard_turn");
    if (target === "send_turn") $("hk-send-btn").textContent = "Press keys…";
    if (target === "discard_turn") $("hk-discard-btn").textContent = "Press keys…";
    send({ action: "set_hotkey_recording", hotkey_recording: true });
  }

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

  function stopSessionReliably() {
    send({ action: "stop_session" });
    fetch("/api/stop", { method: "POST" }).catch(() => {});
  }

  function connectWS() {
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    ws = new WebSocket(`${proto}//${location.host}/ws`);

    ws.onmessage = (evt) => {
      try {
        const msg = JSON.parse(evt.data);
        if (msg.type === "state") {
          state = msg.payload;
          renderState();
        } else if (msg.type === "audio_level") {
          audioLevel = msg.payload.level || 0;
        }
      } catch (e) {
        console.error("WS parse error", e);
      }
    };

    ws.onclose = () => setTimeout(connectWS, 1000);
  }

  async function refreshLogs() {
    try {
      const res = await fetch("/api/logs");
      const data = await res.json();
      $("log-file-path").textContent = data.path || "—";
      const box = $("log-tail-box");
      box.textContent = (data.lines && data.lines.length) ? data.lines.join("\n") : "No log entries yet.";
      box.scrollTop = box.scrollHeight;
    } catch (e) {
      $("log-tail-box").textContent = "Failed to load logs: " + e;
    }
  }

  function openDrawer(tab) {
    activeDrawerTab = tab;
    $("drawer").classList.remove("hidden");
    $("tab-turns").classList.toggle("is-active", tab === "turns");
    $("tab-settings").classList.toggle("is-active", tab === "settings");
    $("panel-turns").classList.toggle("hidden", tab !== "turns");
    $("panel-settings").classList.toggle("hidden", tab !== "settings");
    send({ action: "set_drawer", drawer_open: true });
    if (tab === "settings") refreshLogs();
  }

  function closeDrawer() {
    $("drawer").classList.add("hidden");
    send({ action: "set_drawer", drawer_open: false });
  }

  function renderState() {
    if (!state) return;

    $("hud-timer").textContent = fmtTime(state.elapsed_seconds);
    $("rec-dot").classList.toggle("is-live", !!(state.recording && !state.paused));

    if (state.recording) {
      $("btn-start").classList.add("hidden");
      $("btn-stop").classList.remove("hidden");
      $("btn-pause").disabled = false;
      $("btn-pause").textContent = state.paused ? "▶" : "⏸";
    } else {
      $("btn-start").classList.remove("hidden");
      $("btn-stop").classList.add("hidden");
      $("btn-pause").disabled = true;
      $("btn-pause").textContent = "⏸";
    }

    // Hotkey labels (dynamic from persisted config)
    const hk = state.hotkeys || {
      send_turn: { key_code: "Enter", modifiers: MOD_OPTION, display: "⌥↩" },
      discard_turn: { key_code: "Backspace", modifiers: MOD_OPTION, display: "⌥⌫" },
    };
    const sendLabel = hk.send_turn?.display || "⌥↩";
    const discardLabel = hk.discard_turn?.display || "⌥⌫";
    $("send-kbd").textContent = sendLabel;
    $("btn-send-turn").title = `Finalize & Send Turn to Antigravity (Global ${sendLabel})`;
    $("btn-discard").title = `Discard buffered transcript (Global ${discardLabel})`;
    if (recordingHotkeyTarget !== "send_turn") {
      $("hk-send-btn").textContent = sendLabel;
    }
    if (recordingHotkeyTarget !== "discard_turn") {
      $("hk-discard-btn").textContent = discardLabel;
    }

    // Live transcript (auto-scrolls horizontally to newest spoken words) + highlight Send button
    const hasDraft = !!(state.live_transcript && state.live_transcript.trim().length > 0);
    const txEl = $("live-transcript");
    txEl.textContent = hasDraft
      ? state.live_transcript
      : state.recording
      ? `Speak naturally — press ${sendLabel} when ready to send`
      : `Ready — press Start, speak, then ${sendLabel} to send`;
    txEl.classList.toggle("is-live-draft", hasDraft);
    requestAnimationFrame(() => {
      const overflowed = txEl.scrollWidth > txEl.clientWidth + 2;
      txEl.classList.toggle("is-scrolled", overflowed);
      txEl.scrollLeft = overflowed ? txEl.scrollWidth : 0;
    });

    const sendBtn = $("btn-send-turn");
    sendBtn.classList.toggle("xnk-btn-cobalt", hasDraft);
    sendBtn.classList.toggle("xnk-btn-secondary", !hasDraft);

    // Antigravity Link Status Pill
    const linkBtn = $("btn-link-status");
    const linkText = $("link-pill-text");
    if (state.conversation_id && state.agent_state !== "error") {
      linkBtn.classList.add("is-linked");
      linkBtn.classList.remove("is-unlinked");
      const label = state.linked_repo
        ? state.linked_repo.split("/").pop()
        : state.conversation_id.slice(0, 8);
      linkText.textContent = state.injected_count > 0 ? `${label} (${state.injected_count})` : `Linked: ${label}`;
    } else if (state.conversation_id && state.agent_state === "error") {
      linkBtn.classList.remove("is-linked");
      linkBtn.classList.add("is-unlinked");
      linkText.textContent = "Re-link AGY";
    } else {
      linkBtn.classList.remove("is-linked");
      linkBtn.classList.add("is-unlinked");
      linkText.textContent = "Link AGY";
    }

    // Capture Box readout
    const r = state.region || { width: 1280, height: 720, display_index: 0, locked: false, visible: true };
    $("region-readout").textContent = r.locked
      ? `● LOCKED ${r.width} × ${r.height} · Display ${(r.display_index || 0) + 1}`
      : `${r.width} × ${r.height} · Display ${(r.display_index || 0) + 1}`;
    $("btn-toggle-frame").textContent = r.visible ? "Hide Box" : "Show Box";

    // Settings sync
    if (!$("conv-id-input").matches(":focus")) {
      $("conv-id-input").value = state.conversation_id || "";
    }
    $("top-platform").textContent = state.permissions?.platform || "—";
    if (state.log_file_path) $("log-file-path").textContent = state.log_file_path;

    // Banners
    if (state.last_error) {
      $("error-banner-text").textContent = state.last_error;
      $("error-banner").classList.remove("hidden");
    } else {
      $("error-banner").classList.add("hidden");
    }
    if (state.last_bundle_path && !state.recording) {
      $("bundle-banner-text").textContent = `Archive sealed: ${state.last_bundle_path}`;
      $("bundle-banner").classList.remove("hidden");
    } else {
      $("bundle-banner").classList.add("hidden");
    }

    // Counts, permissions & utterances
    const count = (state.utterances || []).length;
    $("utt-count").textContent = String(count);
    $("utt-count-tab").textContent = String(count);
    renderPermissions(state.permissions);
    renderUtterances(state.utterances || []);
  }

  function renderPermissions(p) {
    if (!p) return;
    const items = [
      { label: "AGENTAPI", ok: p.agy_available },
      { label: "FFMPEG", ok: p.ffmpeg_available },
      { label: "SCREEN REC", ok: p.screen_recording === "granted" },
      { label: "ACCESSIBILITY", ok: p.accessibility === "granted" },
    ];
    $("perm-pills").innerHTML = items
      .map((it) => `<span class="xnk-badge ${it.ok ? "xnk-badge-live" : "xnk-badge-amber"}">${it.label}</span>`)
      .join("");
  }

  function escapeHtml(s) {
    return String(s || "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  function renderUtterances(utts) {
    const list = $("utterance-list");
    if (!utts.length) {
      list.innerHTML = `
        <div class="xnk-empty-state">
          <strong>Partner Mode Ready.</strong><br />
          Run <code>/xnk-iterate</code> in your active Google Antigravity conversation to link, position the detached cyan Capture Box, press <strong>Start</strong>, and talk naturally.
        </div>`;
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
                  `<img class="frame-thumb" src="/api/frame?path=${encodeURIComponent(fp)}" data-frame-label="Utterance #${u.id} ${idx === 0 ? "Trigger" : "Reaction"}" />`
              )
              .join("") +
            `</div>`
          : "";
        return `
          <div class="utt-card ${u.discarded ? "is-discarded" : ""}">
            <div class="utt-meta">
              <span>#${u.id} · ${fmtTime(u.speech_start)}–${fmtTime(u.speech_end)}</span>
              <span>Lookback ${(lb.start_seconds || 0).toFixed(1)}s–${(lb.end_seconds || 0).toFixed(1)}s</span>
            </div>
            <div class="utt-text">${escapeHtml(u.transcript)}</div>
            ${thumbs}
          </div>`;
      })
      .join("");
  }

  function drawWaveforms() {
    waveformHistory.shift();
    const targetVal = state && state.recording && !state.paused
      ? Math.max(0.1, Math.min(1.0, audioLevel + Math.random() * 0.05))
      : 0.08;
    waveformHistory.push(targetVal);

    const canvas = $("hud-waveform");
    if (canvas) {
      const ctx = canvas.getContext("2d");
      const w = canvas.width;
      const h = canvas.height;
      ctx.clearRect(0, 0, w, h);
      const bars = waveformHistory.slice(-3);
      const gap = 2;
      const bw = Math.max(2, Math.floor((w - gap * (bars.length - 1)) / bars.length));
      ctx.fillStyle = "#38bdf8";
      bars.forEach((v, i) => {
        const bh = Math.max(3, v * (h - 2));
        ctx.beginPath();
        ctx.roundRect(i * (bw + gap), (h - bh) / 2, bw, bh, 1.5);
        ctx.fill();
      });
    }
    requestAnimationFrame(drawWaveforms);
  }

  function toggleWebSpeech() {
    const SR = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SR) {
      alert("Web Speech API not supported here; native macOS speech engine is used.");
      return;
    }
    if (webSpeechActive && webSpeech) {
      webSpeechActive = false;
      webSpeech.stop();
      $("btn-web-mic").textContent = "Browser STT";
      return;
    }
    webSpeech = new SR();
    webSpeech.continuous = true;
    webSpeech.interimResults = true;
    webSpeech.lang = "en-US";
    let buf = "";
    webSpeech.onresult = (e) => {
      let interim = "";
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const t = e.results[i][0].transcript;
        if (e.results[i].isFinal) buf = (buf + " " + t).trim();
        else interim += t;
      }
      const disp = (buf + " " + interim).trim();
      if (disp) $("live-transcript").textContent = disp;
    };
    window.__flushWebSpeech = () => {
      if (buf.trim()) {
        send({ action: "inject_utterance", text: buf.trim() });
        buf = "";
      }
    };
    window.__discardWebSpeech = () => { buf = ""; };
    webSpeech.onend = () => { if (webSpeechActive) webSpeech.start(); };
    webSpeechActive = true;
    webSpeech.start();
    $("btn-web-mic").textContent = "🔴 STT On";
  }

  function bindEvents() {
    // Borderless HUD window drag (drag from XNK badge or any non-interactive area of the HUD bar)
    const hudBar = document.querySelector(".hud-bar");
    if (hudBar) {
      hudBar.addEventListener("mousedown", (e) => {
        if (e.button !== 0) return;
        if (e.target.closest("button, input, select, a, pre")) return;
        if (typeof window.__xnkStartDrag === "function") {
          window.__xnkStartDrag();
        }
      });
    }

    $("btn-start").addEventListener("click", () => {
      const dispIdx = state?.region?.display_index || 0;
      send({ action: "start_session", target_id: `display:${dispIdx}` });
    });
    $("btn-stop").addEventListener("click", stopSessionReliably);
    $("btn-pause").addEventListener("click", () => send({ action: "toggle_pause" }));
    $("btn-send-turn").addEventListener("click", () => {
      if (window.__flushWebSpeech) window.__flushWebSpeech();
      send({ action: "send_turn" });
    });
    $("btn-discard").addEventListener("click", () => {
      if (window.__discardWebSpeech) window.__discardWebSpeech();
      send({ action: "discard_turn" });
    });
    $("btn-toggle-frame").addEventListener("click", () => send({ action: "toggle_box_visible" }));
    $("btn-web-mic").addEventListener("click", toggleWebSpeech);

    // Drawer toggles
    $("btn-turns").addEventListener("click", () => {
      if (!$("drawer").classList.contains("hidden") && activeDrawerTab === "turns") closeDrawer();
      else openDrawer("turns");
    });
    $("btn-settings").addEventListener("click", () => {
      if (!$("drawer").classList.contains("hidden") && activeDrawerTab === "settings") closeDrawer();
      else openDrawer("settings");
    });
    $("btn-link-status").addEventListener("click", () => openDrawer("settings"));
    $("btn-close-drawer").addEventListener("click", closeDrawer);
    $("tab-turns").addEventListener("click", () => openDrawer("turns"));
    $("tab-settings").addEventListener("click", () => openDrawer("settings"));

    // Link / Settings actions
    $("btn-bind-conv").addEventListener("click", () => {
      const id = $("conv-id-input").value.trim();
      if (id) send({ action: "bind_conversation", conversation_id: id });
    });
    $("btn-unbind-conv").addEventListener("click", () => send({ action: "unbind_conversation" }));
    $("btn-refresh-logs").addEventListener("click", refreshLogs);
    $("hk-send-btn").addEventListener("click", () => startHotkeyRecording("send_turn"));
    $("hk-discard-btn").addEventListener("click", () => startHotkeyRecording("discard_turn"));
    $("btn-reset-hotkeys").addEventListener("click", () => {
      stopHotkeyRecording();
      send({ action: "reset_hotkeys" });
    });

    // Manual inject
    $("manual-form").addEventListener("submit", (e) => {
      e.preventDefault();
      const input = $("manual-input");
      const text = input.value.trim();
      if (!text) return;
      send({ action: "inject_utterance", text });
      input.value = "";
    });

    // Frame lightbox
    document.addEventListener("click", (e) => {
      const thumb = e.target.closest(".frame-thumb");
      if (thumb) {
        $("frame-modal-img").src = thumb.src;
        $("frame-modal-title").textContent = thumb.dataset.frameLabel || "Frame Snapshot";
        $("frame-modal").classList.remove("hidden");
      }
    });
    $("frame-modal-close").addEventListener("click", () => $("frame-modal").classList.add("hidden"));
    $("frame-modal").addEventListener("click", (e) => { if (e.target === $("frame-modal")) $("frame-modal").classList.add("hidden"); });

    // Hotkeys (shortcut recorder + local fallback when HUD window itself is focused)
    document.addEventListener("keydown", (e) => {
      if (recordingHotkeyTarget) {
        e.preventDefault();
        e.stopPropagation();
        if (e.key === "Escape") {
          stopHotkeyRecording();
          return;
        }
        if (MODIFIER_CODES.has(e.code)) {
          return;
        }
        const mods = eventModifiersMask(e);
        const isFuncKey = /^F([1-9]|1[0-2])$/.test(e.code);
        const hasPrimaryMod = (mods & (MOD_CMD | MOD_OPTION | MOD_CONTROL)) !== 0;
        if (!isFuncKey && !hasPrimaryMod) {
          $("hotkey-hint").textContent = "Include ⌘, ⌥, or ⌃ with your key (or use F1–F12). Press Esc to cancel.";
          return;
        }
        const current = state?.hotkeys || {
          send_turn: { key_code: "Enter", modifiers: MOD_OPTION, display: "⌥↩" },
          discard_turn: { key_code: "Backspace", modifiers: MOD_OPTION, display: "⌥⌫" },
        };
        const next = {
          send_turn: { ...current.send_turn },
          discard_turn: { ...current.discard_turn },
        };
        next[recordingHotkeyTarget] = { key_code: e.code, modifiers: mods, display: "" };
        $("hotkey-hint").textContent = "Click a shortcut to rebind (requires ⌘/⌥/⌃ + key, or F1–F12; Esc cancels).";
        send({ action: "update_hotkeys", hotkeys: next });
        stopHotkeyRecording();
        return;
      }

      const inInput = document.activeElement && ["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement.tagName);
      const hk = state?.hotkeys || {
        send_turn: { key_code: "Enter", modifiers: MOD_OPTION },
        discard_turn: { key_code: "Backspace", modifiers: MOD_OPTION },
      };
      if (matchesHotkey(e, hk.send_turn)) {
        e.preventDefault();
        if (window.__flushWebSpeech) window.__flushWebSpeech();
        send({ action: "send_turn" });
      } else if (matchesHotkey(e, hk.discard_turn)) {
        e.preventDefault();
        if (window.__discardWebSpeech) window.__discardWebSpeech();
        send({ action: "discard_turn" });
      } else if (!inInput && e.code === "Space" && state?.recording) {
        e.preventDefault();
        send({ action: "toggle_pause" });
      } else if (!inInput && e.key === "Escape") {
        if (!$("frame-modal").classList.contains("hidden")) $("frame-modal").classList.add("hidden");
        else if (state?.recording) stopSessionReliably();
      }
    });
  }

  bindEvents();
  connectWS();
  drawWaveforms();
})();
