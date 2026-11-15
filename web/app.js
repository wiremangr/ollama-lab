// Ollama Lab Client Controller

let allModels = [];
let allPresets = [];
let activeControllers = {
  single: null,
  compareA: null,
  compareB: null,
};

// Parameter states
const defaultParams = {
  ctx: 4096,
  predict: 1024,
  temp: 0.70,
  topp: 0.90,
  topk: 40,
  repeat: 1.10,
  seed: null,
};

let singleParams = { ...defaultParams };
let singleKeepAlive = '5m';
let singleThink = true; // Model Thinking (DeepSeek-R1 / CoT)
const conversationHistory = {
  single: [],
  compareA: [],
  compareB: [],
};

function getCompareOutputElement(mode) {
  if (mode === 'compareA') return document.getElementById('output-a');
  if (mode === 'compareB') return document.getElementById('output-b');
  return null;
}

function resetCompareOutputPlaceholder(mode) {
  const outputEl = getCompareOutputElement(mode);
  if (!outputEl) return;
  const text = mode === 'compareA'
    ? 'Awaiting comparison test...'
    : 'Awaiting comparison test...';
  outputEl.innerHTML = `<span class="placeholder-text">${text}</span>`;
}

function setContextRetention(mode, enabled) {
  if (!enabled) {
    conversationHistory[mode] = [];
    if (mode === 'compareA' || mode === 'compareB') {
      resetCompareOutputPlaceholder(mode);
    }
  }
}

function getContextHistory(mode, enabled) {
  return enabled ? conversationHistory[mode] : [];
}

function saveConversationTurn(mode, enabled, userPrompt, assistantReply) {
  if (!enabled || !assistantReply.trim()) return;
  conversationHistory[mode].push(
    { role: 'user', content: userPrompt },
    { role: 'assistant', content: assistantReply },
  );
}

// Initialization on DOM load
document.addEventListener('DOMContentLoaded', () => {
  initTheme();
  checkStatus();
  fetchModels();
  fetchPresets();
  setupKeyboardShortcuts();
  setupSystemPromptCharCounter();
  setupChannelFeedScroll();
  updateActiveAgentsBadge();
  updateChannelRoundInfo();
});

// Theme Management (Dark / Light)
function initTheme() {
  const savedTheme = localStorage.getItem('ollama_lab_theme');
  if (savedTheme === 'light') {
    setTheme(true);
  } else if (savedTheme === 'dark') {
    setTheme(false);
  } else {
    // Check macOS system preference
    const prefersLight = window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches;
    setTheme(prefersLight);
  }
}

function toggleTheme() {
  const isLight = document.body.classList.contains('light-theme');
  setTheme(!isLight);
}

function setTheme(light) {
  const icon = document.getElementById('theme-icon');
  const label = document.getElementById('theme-label');
  if (light) {
    document.body.classList.remove('dark-theme');
    document.body.classList.add('light-theme');
    if (icon) icon.textContent = '🌙';
    if (label) label.textContent = 'Dark';
    localStorage.setItem('ollama_lab_theme', 'light');
  } else {
    document.body.classList.remove('light-theme');
    document.body.classList.add('dark-theme');
    if (icon) icon.textContent = '☀️';
    if (label) label.textContent = 'Light';
    localStorage.setItem('ollama_lab_theme', 'dark');
  }
}

// Model Thinking (DeepSeek-R1 / CoT) Toggle Handler
function onThinkToggle(mode, checked) {
  if (mode === 'single') {
    singleThink = checked;
    const toggle = document.getElementById('single-think-toggle');
    const status = document.getElementById('single-think-status');
    const pill = document.getElementById('single-think-pill');

    if (toggle) toggle.checked = checked;
    if (status) {
      status.textContent = checked ? 'ON' : 'OFF';
      status.className = checked ? 'status-on' : 'status-off';
    }
    if (pill) {
      pill.textContent = checked ? '🧠 Think: ON' : '🧠 Think: OFF';
      pill.className = `chip think-pill ${checked ? 'active' : 'inactive'}`;
    }
  }
}

function toggleThinkPill(mode) {
  if (mode === 'single') {
    onThinkToggle('single', !singleThink);
  }
}

// Keyboard shortcuts (Cmd/Ctrl + Enter, Esc, Ctrl+C/V/A fallback)
function setupKeyboardShortcuts() {
  document.addEventListener('keydown', (e) => {
    // Cmd+Enter or Ctrl+Enter to run
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      const activeTab = document.querySelector('.tab-btn.active')?.id;
      if (activeTab === 'tab-single') {
        runSingleTest();
      } else if (activeTab === 'tab-compare') {
        runCompareTest();
      } else if (activeTab === 'tab-channel') {
        runChannelDiscussion();
      }
    }
    // Esc to stop
    if (e.key === 'Escape') {
      stopGeneration('single');
      stopCompare();
      stopChannel();
    }

    // Ctrl+C / Ctrl+V / Ctrl+A fallback for non-Command keys
    if (e.ctrlKey && !e.metaKey) {
      const active = document.activeElement;
      if (active && (active.tagName === 'INPUT' || active.tagName === 'TEXTAREA')) {
        const key = e.key.toLowerCase();
        if (key === 'a') {
          active.select();
        } else if (key === 'c') {
          const selected = active.value.substring(active.selectionStart, active.selectionEnd);
          if (selected) {
            navigator.clipboard.writeText(selected);
          }
        } else if (key === 'v') {
          navigator.clipboard.readText().then(text => {
            if (text) {
              const start = active.selectionStart;
              const end = active.selectionEnd;
              const val = active.value;
              active.value = val.substring(0, start) + text + val.substring(end);
              active.selectionStart = active.selectionEnd = start + text.length;
              active.dispatchEvent(new Event('input', { bubbles: true }));
            }
          }).catch(() => {});
        }
      }
    }
  });
}

function setupSystemPromptCharCounter() {
  const sysArea = document.getElementById('single-system-prompt');
  const countEl = document.getElementById('single-system-count');
  sysArea.addEventListener('input', () => {
    countEl.textContent = `${sysArea.value.length} chars`;
  });
}

// Mode Switching (Single vs Compare vs Channel)
function switchMode(mode) {
  document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
  document.querySelectorAll('.view-content').forEach(view => view.classList.remove('active'));

  if (mode === 'single') {
    document.getElementById('tab-single')?.classList.add('active');
    document.getElementById('single-view')?.classList.add('active');
  } else if (mode === 'compare') {
    document.getElementById('tab-compare')?.classList.add('active');
    document.getElementById('compare-view')?.classList.add('active');
  } else if (mode === 'channel') {
    document.getElementById('tab-channel')?.classList.add('active');
    document.getElementById('channel-view')?.classList.add('active');
  }
}

// Check Ollama Connection Status
async function checkStatus() {
  const statusEl = document.getElementById('ollama-status');
  const statusText = document.getElementById('status-text');
  const offlineBanner = document.getElementById('offline-banner');

  try {
    const res = await fetch('/api/status');
    const data = await res.json();
    if (data.connected) {
      statusEl.className = 'status-indicator connected';
      statusText.textContent = `Ollama ${data.version || 'Connected'}`;
      offlineBanner.classList.add('hidden');
    } else {
      statusEl.className = 'status-indicator error';
      statusText.textContent = 'Ollama Offline';
      document.getElementById('banner-url').textContent = data.url;
      offlineBanner.classList.remove('hidden');
    }
  } catch (err) {
    statusEl.className = 'status-indicator error';
    statusText.textContent = 'Connection Error';
    offlineBanner.classList.remove('hidden');
  }
}

// Fetch Installed Models from Ollama
async function fetchModels() {
  const modelSelect = document.getElementById('single-model');
  const compareSelectA = document.getElementById('compare-model-a');
  const compareSelectB = document.getElementById('compare-model-b');
  const channelSelect = document.getElementById('channel-model');
  const agent1Select = document.getElementById('agent1-model');
  const agent2Select = document.getElementById('agent2-model');
  const agent3Select = document.getElementById('agent3-model');

  try {
    const res = await fetch('/api/models');
    if (!res.ok) throw new Error('Failed to fetch models');
    allModels = await res.json();

    if (!allModels || allModels.length === 0) {
      const emptyOpt = '<option value="" disabled selected>No models found in Ollama</option>';
      if (modelSelect) modelSelect.innerHTML = emptyOpt;
      if (compareSelectA) compareSelectA.innerHTML = emptyOpt;
      if (compareSelectB) compareSelectB.innerHTML = emptyOpt;
      if (channelSelect) channelSelect.innerHTML = emptyOpt;
      return;
    }

    const optionsHtml = allModels.map(m => {
      const sizeGb = (m.size / (1024 * 1024 * 1024)).toFixed(1);
      const details = m.details?.parameter_size ? `(${m.details.parameter_size}, ${m.details.quantization_level || 'GGUF'})` : `(${sizeGb} GB)`;
      return `<option value="${m.name}">${m.name} ${details}</option>`;
    }).join('');

    if (modelSelect) modelSelect.innerHTML = optionsHtml;
    if (compareSelectA) compareSelectA.innerHTML = optionsHtml;
    if (compareSelectB) compareSelectB.innerHTML = optionsHtml;
    if (channelSelect) channelSelect.innerHTML = optionsHtml;

    const agentOptHtml = '<option value="">(Inherit Model)</option>' + allModels.map(m => `<option value="${m.name}">${m.name}</option>`).join('');
    document.querySelectorAll('.agent-model-select').forEach(sel => {
      const currentVal = sel.value;
      sel.innerHTML = agentOptHtml;
      if (currentVal) sel.value = currentVal;
    });

    if (allModels.length > 1) {
      if (compareSelectB) compareSelectB.selectedIndex = 1;
    }

    updateModelMeta();
    if (modelSelect) modelSelect.onchange = updateModelMeta;

  } catch (err) {
    console.error('Error loading models:', err);
    if (modelSelect) modelSelect.innerHTML = '<option value="" disabled selected>Failed to load models</option>';
  }
}

function updateModelMeta() {
  const modelName = document.getElementById('single-model').value;
  const metaEl = document.getElementById('model-details-badge');
  const model = allModels.find(m => m.name === modelName);
  if (model && model.details) {
    metaEl.textContent = `Family: ${model.details.family || 'unknown'} | Size: ${model.details.parameter_size || 'N/A'} | Quant: ${model.details.quantization_level || 'N/A'}`;
  } else {
    metaEl.textContent = '';
  }
}

// Fetch System Prompt Presets
async function fetchPresets() {
  const presetSelect = document.getElementById('single-preset');
  try {
    const res = await fetch('/api/presets');
    allPresets = await res.json();

    let html = '<option value="">Custom (Manual)</option>';
    allPresets.forEach(p => {
      html += `<option value="${p.id}">${p.name}</option>`;
    });
    presetSelect.innerHTML = html;
  } catch (err) {
    console.error('Error loading presets:', err);
  }
}

// Apply selected preset
function applyPreset(mode) {
  const presetId = document.getElementById(`${mode}-preset`).value;
  const preset = allPresets.find(p => p.id === presetId);
  if (!preset) return;

  document.getElementById(`${mode}-system-prompt`).value = preset.system_prompt;
  document.getElementById(`${mode}-system-count`).textContent = `${preset.system_prompt.length} chars`;

  if (preset.options) {
    if (preset.options.num_ctx !== undefined) setParam(mode, 'ctx', preset.options.num_ctx);
    if (preset.options.num_predict !== undefined) setParam(mode, 'predict', preset.options.num_predict);
    if (preset.options.temperature !== undefined) setParam(mode, 'temp', preset.options.temperature);
    if (preset.options.top_p !== undefined) setParam(mode, 'topp', preset.options.top_p);
  }
}

// Keep-Alive Handler
function setKeepAlive(mode, val) {
  singleKeepAlive = val;
  const label = document.getElementById(`${mode}-keepalive-val`);
  if (label) {
    label.textContent = val === '-1' ? 'Always (-1)' : val;
  }

  const select = document.getElementById(`${mode}-keepalive-select`);
  if (select) select.value = val;

  // Update chips
  const group = label.closest('.param-group');
  if (group) {
    group.querySelectorAll('.chip').forEach(chip => {
      const match = (val === '5m' && chip.textContent.startsWith('5m')) ||
                    (val === '15m' && chip.textContent.startsWith('15m')) ||
                    (val === '30m' && chip.textContent.startsWith('30m')) ||
                    (val === '1h' && chip.textContent.startsWith('1h')) ||
                    (val === '-1' && chip.textContent.startsWith('Always'));
      chip.classList.toggle('active', match);
    });
  }
}

function onKeepAliveSelect(mode, val) {
  setKeepAlive(mode, val);
}

// Parameter Slider & Input Handlers
function onParamSlider(mode, key, val) {
  val = parseFloat(val);
  setParam(mode, key, val, false);
}

function onParamInput(mode, key, val) {
  val = val === '' ? null : parseFloat(val);
  singleParams[key] = val;
}

function setParam(mode, key, val, updateInput = true) {
  singleParams[key] = val;

  // Update label
  const labelEl = document.getElementById(`${mode}-${key}-val`);
  if (labelEl) {
    if (key === 'temp' || key === 'topp' || key === 'repeat') {
      labelEl.textContent = Number(val).toFixed(2);
    } else {
      labelEl.textContent = val === -1 ? 'Max (-1)' : val;
    }
  }

  // Update slider/input if called from chip
  if (updateInput) {
    const slider = document.getElementById(`${mode}-${key}-slider`);
    if (slider && val !== -1) slider.value = val;
  }

  // Update chip active states for ctx & predict
  if (key === 'ctx' || key === 'predict') {
    const group = document.getElementById(`${mode}-${key}-val`).closest('.param-group');
    group.querySelectorAll('.chip').forEach(chip => {
      const chipText = chip.textContent.trim();
      let match = false;
      if (key === 'ctx') {
        if (val === 2048 && chipText === '2K') match = true;
        if (val === 4096 && chipText === '4K') match = true;
        if (val === 8192 && chipText === '8K') match = true;
        if (val === 16384 && chipText === '16K') match = true;
        if (val === 32768 && chipText === '32K') match = true;
      } else if (key === 'predict') {
        if (val === 256 && chipText === '256') match = true;
        if (val === 512 && chipText === '512') match = true;
        if (val === 1024 && chipText === '1K') match = true;
        if (val === 2048 && chipText === '2K') match = true;
        if (val === -1 && chipText.startsWith('Max')) match = true;
      }
      chip.classList.toggle('active', match);
    });
  }
}

function resetParams(mode) {
  singleParams = { ...defaultParams };
  setParam(mode, 'ctx', defaultParams.ctx);
  setParam(mode, 'predict', defaultParams.predict);
  setParam(mode, 'temp', defaultParams.temp);
  setParam(mode, 'topp', defaultParams.topp);
  setParam(mode, 'repeat', defaultParams.repeat);
  document.getElementById(`${mode}-topk-input`).value = defaultParams.topk;
  document.getElementById(`${mode}-seed-input`).value = '';
  setKeepAlive('single', '5m');
  onThinkToggle('single', true);
}

function insertSamplePrompt(text) {
  const area = document.getElementById('single-user-prompt');
  area.value = text;
  area.focus();
}

// ========================================================
// StreamRenderer: Handles Live Thinking + Final Answer
// ========================================================
class StreamRenderer {
  constructor(container, clearContainer = true) {
    this.container = container;
    if (clearContainer) {
      this.container.innerHTML = '';
    }
    this.thoughtBox = null;
    this.thoughtContent = null;
    this.thoughtStatus = null;
    this.responseBox = document.createElement('div');
    this.responseBox.className = 'final-response';
    this.container.appendChild(this.responseBox);

    this.rawBuffer = '';
    this.isInsideThinkTag = false;
  }

  ensureThoughtBox() {
    if (!this.thoughtBox) {
      this.thoughtBox = document.createElement('details');
      this.thoughtBox.className = 'thought-container';
      this.thoughtBox.open = true;

      const summary = document.createElement('summary');
      summary.className = 'thought-summary';
      summary.innerHTML = `
        <span class="thought-icon">🧠</span>
        <span class="thought-title">Thinking Process</span>
        <span class="thought-status-badge">Thinking...</span>
      `;
      this.thoughtBox.appendChild(summary);

      this.thoughtStatus = summary.querySelector('.thought-status-badge');

      this.thoughtContent = document.createElement('div');
      this.thoughtContent.className = 'thought-content';
      this.thoughtBox.appendChild(this.thoughtContent);

      this.container.insertBefore(this.thoughtBox, this.responseBox);
    }
  }

  append(delta, thinking) {
    // 1. Separate thinking field from Ollama API
    if (thinking) {
      this.ensureThoughtBox();
      this.thoughtContent.textContent += thinking;
      this.thoughtContent.scrollTop = this.thoughtContent.scrollHeight;
      return;
    }

    if (!delta) return;

    // 2. Parse inline <think> tags if present in delta
    this.rawBuffer += delta;

    while (this.rawBuffer.length > 0) {
      if (!this.isInsideThinkTag) {
        const thinkStartIdx = this.rawBuffer.indexOf('<think>');
        if (thinkStartIdx !== -1) {
          const before = this.rawBuffer.slice(0, thinkStartIdx);
          if (before) this.responseBox.textContent += before;
          this.ensureThoughtBox();
          this.isInsideThinkTag = true;
          this.rawBuffer = this.rawBuffer.slice(thinkStartIdx + 7);
        } else {
          // Check if partial <think> is arriving
          if (this.rawBuffer.endsWith('<') || this.rawBuffer.endsWith('<t') || this.rawBuffer.endsWith('<th') || this.rawBuffer.endsWith('<thi') || this.rawBuffer.endsWith('<thin') || this.rawBuffer.endsWith('<think')) {
            break;
          }
          this.responseBox.textContent += this.rawBuffer;
          this.rawBuffer = '';
        }
      } else {
        const thinkEndIdx = this.rawBuffer.indexOf('</think>');
        if (thinkEndIdx !== -1) {
          const thinkText = this.rawBuffer.slice(0, thinkEndIdx);
          this.thoughtContent.textContent += thinkText;
          this.isInsideThinkTag = false;
          if (this.thoughtStatus) this.thoughtStatus.textContent = 'Completed';
          this.rawBuffer = this.rawBuffer.slice(thinkEndIdx + 8);
        } else {
          if (this.rawBuffer.endsWith('<') || this.rawBuffer.endsWith('</') || this.rawBuffer.endsWith('</t') || this.rawBuffer.endsWith('</th') || this.rawBuffer.endsWith('</thi') || this.rawBuffer.endsWith('</thin') || this.rawBuffer.endsWith('</think')) {
            break;
          }
          this.thoughtContent.textContent += this.rawBuffer;
          this.thoughtContent.scrollTop = this.thoughtContent.scrollHeight;
          this.rawBuffer = '';
        }
      }
    }

    this.container.scrollTop = this.container.scrollHeight;
    const scrollParent = this.container.closest('.output-body, .channel-feed-body');
    if (scrollParent) {
      scrollParent.scrollTop = scrollParent.scrollHeight;
    }
  }

  finish(totalSec) {
    if (this.rawBuffer) {
      if (this.isInsideThinkTag) {
        if (this.thoughtContent) this.thoughtContent.textContent += this.rawBuffer;
      } else {
        this.responseBox.textContent += this.rawBuffer;
      }
      this.rawBuffer = '';
    }

    if (this.thoughtBox && this.thoughtStatus) {
      this.thoughtStatus.textContent = totalSec ? `Done (${totalSec.toFixed(1)}s)` : 'Done';
    }
  }

  error(msg) {
    const errDiv = document.createElement('div');
    errDiv.style.color = 'var(--danger)';
    errDiv.style.marginTop = '10px';
    errDiv.textContent = `[Error: ${msg}]`;
    this.container.appendChild(errDiv);
  }
}

// Run Single Test
async function runSingleTest() {
  const model = document.getElementById('single-model').value;
  if (!model) {
    alert('Please select a model first!');
    return;
  }

  const systemPrompt = document.getElementById('single-system-prompt').value;
  const userPrompt = document.getElementById('single-user-prompt').value;
  const keepContext = document.getElementById('single-keep-context').checked;
  if (!userPrompt.trim()) {
    alert('Please enter a user prompt to test!');
    return;
  }

  const runBtn = document.getElementById('single-run-btn');
  const stopBtn = document.getElementById('single-stop-btn');
  const streamBadge = document.getElementById('single-stream-badge');
  const outputEl = document.getElementById('single-output-text');

  // Prepare options
  const topkVal = parseInt(document.getElementById('single-topk-input').value, 10);
  const seedInput = document.getElementById('single-seed-input').value;
  const seedVal = seedInput !== '' ? parseInt(seedInput, 10) : null;

  const options = {
    num_ctx: singleParams.ctx,
    num_predict: singleParams.predict,
    temperature: singleParams.temp,
    top_p: singleParams.topp,
    top_k: isNaN(topkVal) ? 40 : topkVal,
    repeat_penalty: singleParams.repeat,
  };
  if (seedVal !== null && !isNaN(seedVal)) {
    options.seed = seedVal;
  }

  let rendererTarget = outputEl;
  let turnMetaEl = null;

  if (keepContext) {
    if (conversationHistory.single.length === 0) {
      outputEl.innerHTML = '';
    }

    // Append User turn block
    const userTurn = document.createElement('div');
    userTurn.className = 'dialogue-turn user-turn';
    const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    userTurn.innerHTML = `
      <div class="turn-header">
        <span class="turn-author">👤 You</span>
        <span class="turn-meta">${timeStr}</span>
      </div>
      <div class="turn-body">${escapeHtml(userPrompt)}</div>
    `;
    outputEl.appendChild(userTurn);

    // Append Assistant turn block
    const assistantTurn = document.createElement('div');
    assistantTurn.className = 'dialogue-turn assistant-turn';
    const turnId = Date.now();
    assistantTurn.innerHTML = `
      <div class="turn-header">
        <span class="turn-author">🤖 ${escapeHtml(model)}</span>
        <span class="turn-meta" id="single-turn-meta-${turnId}">Streaming...</span>
      </div>
      <div class="turn-body" id="single-turn-body-${turnId}"></div>
    `;
    outputEl.appendChild(assistantTurn);
    rendererTarget = document.getElementById(`single-turn-body-${turnId}`);
    turnMetaEl = document.getElementById(`single-turn-meta-${turnId}`);

    // Clear input field for next turn
    document.getElementById('single-user-prompt').value = '';
  } else {
    outputEl.innerHTML = '';
  }

  // Reset output and UI states
  const renderer = new StreamRenderer(rendererTarget, !keepContext);
  streamBadge.classList.remove('hidden');
  runBtn.classList.add('hidden');
  stopBtn.classList.remove('hidden');
  resetMetrics('single');

  const controller = new AbortController();
  activeControllers.single = controller;
  let assistantReply = '';

  try {
    const response = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        model,
        system_prompt: systemPrompt,
        user_prompt: userPrompt,
        history: getContextHistory('single', keepContext),
        think: singleThink,
        keep_alive: singleKeepAlive,
        options,
      }),
      signal: controller.signal,
    });

    if (!response.ok) {
      throw new Error(`Server returned error ${response.status}`);
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder('utf-8');
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n\n');
      buffer = lines.pop(); // Keep uncompleted tail

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        const jsonStr = line.slice(6);
        try {
          const chunk = JSON.parse(jsonStr);
          if (chunk.error) {
            renderer.error(chunk.error);
          } else {
            renderer.append(chunk.delta, chunk.thinking);
            assistantReply += chunk.delta || '';
          }

          if (chunk.done && chunk.metrics) {
            updateMetrics('single', chunk.metrics);
            renderer.finish(chunk.metrics.total_duration_ms / 1000);
            if (turnMetaEl) {
              turnMetaEl.textContent = `${chunk.metrics.output_tokens_per_s.toFixed(1)} tok/s • ${chunk.metrics.output_tokens} tok • ${(chunk.metrics.total_duration_ms / 1000).toFixed(2)}s`;
            }
          }
        } catch (e) {
          console.error('Failed to parse SSE chunk', e);
        }
      }
      outputEl.scrollTop = outputEl.scrollHeight;
    }
  } catch (err) {
    if (err.name === 'AbortError') {
      renderer.append('\n\n[⏹ Test stopped by user]');
      if (turnMetaEl) turnMetaEl.textContent = 'Stopped';
    } else {
      renderer.error(err.message);
      if (turnMetaEl) turnMetaEl.textContent = 'Error';
    }
  } finally {
    streamBadge.classList.add('hidden');
    runBtn.classList.remove('hidden');
    stopBtn.classList.add('hidden');
    activeControllers.single = null;
    if (!controller.signal.aborted) {
      saveConversationTurn('single', keepContext, userPrompt, assistantReply);
    }
    outputEl.scrollTop = outputEl.scrollHeight;
  }
}

function stopGeneration(mode) {
  if (activeControllers[mode]) {
    activeControllers[mode].abort();
    activeControllers[mode] = null;
  }
}

// Update Single Metrics Cards
function updateMetrics(prefix, metrics) {
  document.getElementById(`${prefix}-speed-metric`).innerHTML = `${metrics.output_tokens_per_s.toFixed(1)} <small>tok/s</small>`;
  document.getElementById(`${prefix}-ttft-metric`).innerHTML = `${metrics.time_to_first_token_ms.toFixed(0)} <small>ms</small>`;
  document.getElementById(`${prefix}-duration-metric`).innerHTML = `${(metrics.total_duration_ms / 1000).toFixed(2)} <small>s</small>`;
  document.getElementById(`${prefix}-tokens-metric`).textContent = `${metrics.output_tokens} out / ${metrics.prompt_tokens} in`;
  document.getElementById(`${prefix}-prompt-speed-metric`).innerHTML = `${metrics.prompt_tokens_per_s.toFixed(0)} <small>tok/s</small>`;
}

function resetMetrics(prefix) {
  document.getElementById(`${prefix}-speed-metric`).innerHTML = `-- <small>tok/s</small>`;
  document.getElementById(`${prefix}-ttft-metric`).innerHTML = `-- <small>ms</small>`;
  document.getElementById(`${prefix}-duration-metric`).innerHTML = `-- <small>s</small>`;
  document.getElementById(`${prefix}-tokens-metric`).textContent = `--`;
  document.getElementById(`${prefix}-prompt-speed-metric`).innerHTML = `-- <small>tok/s</small>`;
}

// A/B Comparison Runner
async function runCompareTest() {
  const userPrompt = document.getElementById('compare-user-prompt').value;
  if (!userPrompt.trim()) {
    alert('Please enter a test prompt for comparison!');
    return;
  }

  const modelA = document.getElementById('compare-model-a').value;
  const modelB = document.getElementById('compare-model-b').value;
  if (!modelA || !modelB) {
    alert('Please select both models for comparison!');
    return;
  }

  const sysA = document.getElementById('compare-system-a').value;
  const sysB = document.getElementById('compare-system-b').value;

  const ctxA = parseInt(document.getElementById('compare-ctx-a').value, 10) || 4096;
  const ctxB = parseInt(document.getElementById('compare-ctx-b').value, 10) || 4096;

  const predA = parseInt(document.getElementById('compare-predict-a').value, 10) || 1024;
  const predB = parseInt(document.getElementById('compare-predict-b').value, 10) || 1024;

  const tempA = parseFloat(document.getElementById('compare-temp-a').value) || 0.7;
  const tempB = parseFloat(document.getElementById('compare-temp-b').value) || 0.7;

  const keepAliveA = document.getElementById('compare-keepalive-a').value || '1h';
  const keepAliveB = document.getElementById('compare-keepalive-b').value || '1h';

  const thinkA = document.getElementById('compare-think-a').checked;
  const thinkB = document.getElementById('compare-think-b').checked;
  const keepContextA = document.getElementById('compare-keep-context-a').checked;
  const keepContextB = document.getElementById('compare-keep-context-b').checked;

  // Toggle buttons
  document.getElementById('compare-run-btn').classList.add('hidden');
  document.getElementById('compare-stop-btn').classList.remove('hidden');

  // Prepare outputs per side
  const outputA = document.getElementById('output-a');
  const outputB = document.getElementById('output-b');
  if (!keepContextA) {
    outputA.innerHTML = '';
  } else {
    const placeholderA = outputA.querySelector('.placeholder-text');
    if (placeholderA) placeholderA.remove();
  }
  if (!keepContextB) {
    outputB.innerHTML = '';
  } else {
    const placeholderB = outputB.querySelector('.placeholder-text');
    if (placeholderB) placeholderB.remove();
  }
  document.getElementById('stream-badge-a').classList.remove('hidden');
  document.getElementById('stream-badge-b').classList.remove('hidden');

  // Spawn two parallel streams
  const pA = runStream('compareA', 'output-a', 'stream-badge-a', 'metric-speed-a', 'metric-ttft-a', 'metric-tokens-a', {
    model: modelA,
    system_prompt: sysA,
    user_prompt: userPrompt,
    history: getContextHistory('compareA', keepContextA),
    think: thinkA,
    keep_alive: keepAliveA,
    options: { num_ctx: ctxA, num_predict: predA, temperature: tempA },
  }, keepContextA, 'Candidate A');

  const pB = runStream('compareB', 'output-b', 'stream-badge-b', 'metric-speed-b', 'metric-ttft-b', 'metric-tokens-b', {
    model: modelB,
    system_prompt: sysB,
    user_prompt: userPrompt,
    history: getContextHistory('compareB', keepContextB),
    think: thinkB,
    keep_alive: keepAliveB,
    options: { num_ctx: ctxB, num_predict: predB, temperature: tempB },
  }, keepContextB, 'Candidate B');

  await Promise.allSettled([pA, pB]);

  if (keepContextA || keepContextB) {
    document.getElementById('compare-user-prompt').value = '';
  }

  document.getElementById('compare-run-btn').classList.remove('hidden');
  document.getElementById('compare-stop-btn').classList.add('hidden');
}

async function runStream(slotKey, outputId, badgeId, speedId, ttftId, tokensId, payload, keepContext, candidateLabel = '') {
  const controller = new AbortController();
  activeControllers[slotKey] = controller;
  const outputEl = document.getElementById(outputId);
  const badgeEl = document.getElementById(badgeId);

  let renderer;
  let turnMetaEl = null;
  let assistantReply = '';

  if (keepContext) {
    const placeholder = outputEl.querySelector('.placeholder-text');
    if (placeholder) placeholder.remove();

    const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    const turnId = `${slotKey}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;

    const userTurn = document.createElement('div');
    userTurn.className = 'dialogue-turn user-turn';
    userTurn.innerHTML = `
      <div class="turn-header">
        <span class="turn-author">👤 You</span>
        <span class="turn-meta">${timeStr}</span>
      </div>
      <div class="turn-body">${escapeHtml(payload.user_prompt)}</div>
    `;
    outputEl.appendChild(userTurn);

    const assistantTurn = document.createElement('div');
    assistantTurn.className = 'dialogue-turn assistant-turn';
    const agentLabel = candidateLabel ? `${candidateLabel} • ${payload.model}` : (payload.model || 'Assistant');
    assistantTurn.innerHTML = `
      <div class="turn-header">
        <span class="turn-author">🤖 ${escapeHtml(agentLabel)}</span>
        <span class="turn-meta" id="compare-turn-meta-${turnId}">Streaming...</span>
      </div>
      <div class="turn-body" id="compare-turn-body-${turnId}"></div>
    `;
    outputEl.appendChild(assistantTurn);

    const turnBody = document.getElementById(`compare-turn-body-${turnId}`);
    turnMetaEl = document.getElementById(`compare-turn-meta-${turnId}`);
    renderer = new StreamRenderer(turnBody, false);
    outputEl.scrollTop = outputEl.scrollHeight;
  } else {
    outputEl.innerHTML = '';
    renderer = new StreamRenderer(outputEl);
  }

  try {
    const response = await fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      signal: controller.signal,
    });

    if (!response.ok) {
      throw new Error(`Server returned error ${response.status}`);
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder('utf-8');
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n\n');
      buffer = lines.pop();

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        try {
          const chunk = JSON.parse(line.slice(6));
          if (chunk.error) {
            renderer.error(chunk.error);
          } else {
            renderer.append(chunk.delta, chunk.thinking);
            assistantReply += chunk.delta || '';
          }

          if (chunk.done && chunk.metrics) {
            document.getElementById(speedId).textContent = `${chunk.metrics.output_tokens_per_s.toFixed(1)} tok/s`;
            document.getElementById(ttftId).textContent = `${chunk.metrics.time_to_first_token_ms.toFixed(0)} ms`;
            document.getElementById(tokensId).textContent = `${chunk.metrics.output_tokens} tok`;
            renderer.finish(chunk.metrics.total_duration_ms / 1000);
            if (turnMetaEl) {
              turnMetaEl.textContent = `${chunk.metrics.output_tokens_per_s.toFixed(1)} tok/s • ${chunk.metrics.output_tokens} tok • ${(chunk.metrics.total_duration_ms / 1000).toFixed(2)}s`;
            }
          }
        } catch (e) {}
      }
      if (keepContext) {
        outputEl.scrollTop = outputEl.scrollHeight;
      }
    }
  } catch (err) {
    if (err.name === 'AbortError') {
      renderer.append('\n\n[⏹ Stopped]');
      if (turnMetaEl) turnMetaEl.textContent = 'Stopped';
    } else {
      renderer.error(err.message);
      if (turnMetaEl) turnMetaEl.textContent = 'Error';
    }
  } finally {
    badgeEl.classList.add('hidden');
    activeControllers[slotKey] = null;
    if (!controller.signal.aborted) {
      saveConversationTurn(slotKey, keepContext, payload.user_prompt, assistantReply);
    }
    if (keepContext) {
      outputEl.scrollTop = outputEl.scrollHeight;
    }
  }
}

function stopCompare() {
  stopGeneration('compareA');
  stopGeneration('compareB');
  document.getElementById('compare-run-btn').classList.remove('hidden');
  document.getElementById('compare-stop-btn').classList.add('hidden');
}

// Helpers
function clearOutput(mode) {
  if (mode === 'single') {
    conversationHistory.single = [];
    document.getElementById('single-output-text').innerHTML = '<span class="placeholder-text">Output will appear here in real time as tokens are generated...</span>';
    resetMetrics('single');
  } else {
    document.getElementById(`${mode}-output-text`).innerHTML = '<span class="placeholder-text">Cleared. Awaiting run...</span>';
    resetMetrics(mode);
  }
}

function copyOutput(elementId) {
  const el = document.getElementById(elementId);
  if (!el) return;

  // Check if dialogue turns exist (Single Test conversation mode)
  const turns = el.querySelectorAll('.dialogue-turn');
  if (turns && turns.length > 0) {
    let transcript = '';
    turns.forEach(turn => {
      const author = turn.querySelector('.turn-author')?.textContent.trim() || '';
      const body = turn.querySelector('.turn-body')?.innerText || turn.querySelector('.turn-body')?.textContent || '';
      transcript += `${author}:\n${body.trim()}\n\n`;
    });
    navigator.clipboard.writeText(transcript.trim());
    alert('Copied conversation transcript to clipboard!');
    return;
  }

  // If there's a final response box, copy that or full text
  const text = el.innerText || el.textContent;
  if (!text) return;
  navigator.clipboard.writeText(text.trim());
  alert('Copied output to clipboard!');
}

// Preset Modal
function openSavePresetModal() {
  const prompt = document.getElementById('single-system-prompt').value;
  if (!prompt.trim()) {
    alert('Please enter a system prompt first to save as a preset!');
    return;
  }
  document.getElementById('preset-prompt-preview').textContent = prompt;
  document.getElementById('preset-name-input').value = '';
  document.getElementById('preset-desc-input').value = '';
  document.getElementById('preset-modal').classList.remove('hidden');
  document.getElementById('preset-name-input').focus();
}

function closePresetModal() {
  document.getElementById('preset-modal').classList.add('hidden');
}

async function confirmSavePreset() {
  const name = document.getElementById('preset-name-input').value.trim();
  const desc = document.getElementById('preset-desc-input').value.trim();
  const prompt = document.getElementById('single-system-prompt').value;

  if (!name) {
    alert('Please enter a name for the preset.');
    return;
  }

  const preset = {
    name,
    description: desc,
    system_prompt: prompt,
    options: {
      num_ctx: singleParams.ctx,
      num_predict: singleParams.predict,
      temperature: singleParams.temp,
      top_p: singleParams.topp,
    },
  };

  try {
    const res = await fetch('/api/presets', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(preset),
    });
    if (!res.ok) throw new Error('Failed to save preset');
    closePresetModal();
    await fetchPresets();
  } catch (err) {
    alert('Error saving preset: ' + err.message);
  }
}

// ========================================================
// CHANNEL MODE: Multi-Agent Collaborative Discussion
// ========================================================
let channelHistory = [];
let isChannelRunning = false;
let channelAbortController = null;
let lastFirstAgentId = null; // Strictly prevents the same agent from speaking first 2 rounds in a row!

function escapeHtml(text) {
  if (!text) return '';
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function onAgentToggle(agentId, checked) {
  const card = document.getElementById('agent-card-' + agentId);
  if (card) {
    card.classList.toggle('card-disabled', !checked);
  }
  updateActiveAgentsBadge();
  updateChannelRoundInfo();
}

function getAllAgents() {
  const cards = document.querySelectorAll('.agent-config-card');
  const baseModel = document.getElementById('channel-model')?.value || '';
  const agents = [];

  cards.forEach((card, idx) => {
    const id = card.dataset.agentId ? parseInt(card.dataset.agentId, 10) : (idx + 1);
    const enabledInput = card.querySelector('.agent-enable-switch input');
    const isEnabled = enabledInput ? enabledInput.checked : true;
    const nameInput = card.querySelector('.agent-name-input');
    const promptArea = card.querySelector('.agent-textarea');
    const modelSelect = card.querySelector('.agent-model-select');
    const colorNum = ((id - 1) % 5) + 1;

    agents.push({
      id: id,
      index: idx,
      name: nameInput?.value.trim() || `Agent ${id}`,
      prompt: promptArea?.value.trim() || '',
      model: modelSelect?.value || baseModel,
      enabled: isEnabled,
      cardClass: `agent-${colorNum}-card`,
      pillClass: `pill-agent-${colorNum}`,
      badgeClass: `badge-agent-${colorNum}`,
      borderClass: `agent-${colorNum}-border`,
    });
  });

  return agents;
}

function getEnabledAgents() {
  return getAllAgents().filter(a => a.enabled);
}

function updateActiveAgentsBadge() {
  const all = getAllAgents();
  const enabled = all.filter(a => a.enabled);
  const badge = document.getElementById('channel-active-count-badge');
  if (badge) {
    badge.textContent = `${enabled.length}/${all.length} Active`;
    badge.classList.toggle('warning', enabled.length === 0);
  }
}

function addNewAgent() {
  const panel = document.getElementById('channel-agents-panel');
  if (!panel) return;

  const all = getAllAgents();
  const nextId = all.length > 0 ? Math.max(...all.map(a => a.id)) + 1 : 1;
  const colorNum = ((nextId - 1) % 5) + 1;

  let modelOpts = '<option value="">(Inherit Model)</option>';
  if (allModels && allModels.length > 0) {
    modelOpts += allModels.map(m => `<option value="${m.name}">${m.name}</option>`).join('');
  }

  const card = document.createElement('div');
  card.className = `agent-config-card agent-${colorNum}-border`;
  card.id = `agent-card-${nextId}`;
  card.dataset.agentId = nextId;
  card.innerHTML = `
    <div class="agent-card-header">
      <div class="agent-identity">
        <label class="switch switch-sm agent-enable-switch" title="Enable/Disable Agent ${nextId}">
          <input type="checkbox" id="agent${nextId}-enabled" checked onchange="onAgentToggle(${nextId}, this.checked)">
          <span class="slider round"></span>
        </label>
        <span class="agent-badge badge-agent-${colorNum}">${nextId}</span>
        <input type="text" id="agent${nextId}-name" class="agent-name-input" value="Agent ${nextId}" oninput="updateActiveAgentsBadge()">
      </div>
      <div class="agent-header-actions">
        <select id="agent${nextId}-model" class="mini-select agent-model-select" title="Specific model or inherit channel model">
          ${modelOpts}
        </select>
        <button type="button" class="agent-delete-btn" onclick="removeAgent(${nextId})" title="Remove Agent">✕</button>
      </div>
    </div>
    <textarea id="agent${nextId}-prompt" class="prompt-textarea agent-textarea" rows="3" placeholder="Agent ${nextId} System Prompt...">You are Agent ${nextId}. Contribute your unique perspective to the channel discussion constructively.</textarea>
  `;

  panel.appendChild(card);
  updateActiveAgentsBadge();
  updateChannelRoundInfo();
}

function removeAgent(agentId) {
  const card = document.getElementById(`agent-card-${agentId}`);
  if (card) {
    card.remove();
    updateActiveAgentsBadge();
    updateChannelRoundInfo();
  }
}

function setChannelRounds(n) {
  const input = document.getElementById('channel-rounds');
  if (input) {
    input.value = n;
  }
  const chips = input?.closest('.toolbar-field')?.querySelectorAll('.chip');
  if (chips) {
    chips.forEach(chip => {
      chip.classList.toggle('active', chip.textContent.trim() === String(n));
    });
  }
  updateChannelRoundInfo();
}

function updateChannelRoundInfo() {
  const rounds = parseInt(document.getElementById('channel-rounds')?.value, 10) || 1;
  const order = document.getElementById('channel-order')?.value || 'forward';
  const infoEl = document.getElementById('channel-round-info');
  if (!infoEl) return;

  const enabled = getEnabledAgents();
  let orderName = 'Forward';
  if (order === 'reverse') orderName = 'Reverse';
  else if (order === 'random') orderName = 'Random (No Repeat 1st)';

  infoEl.textContent = `${rounds} cycle${rounds > 1 ? 's' : ''} • ${enabled.length} Active Agent${enabled.length !== 1 ? 's' : ''} • ${orderName}`;
}

function toggleAgentsPanel() {
  const panel = document.getElementById('channel-agents-panel');
  const btn = document.getElementById('toggle-agents-panel-btn');
  if (!panel) return;
  panel.classList.toggle('collapsed');
  const isCollapsed = panel.classList.contains('collapsed');
  if (btn) {
    btn.textContent = isCollapsed ? '⚙️ Agents Setup ▸' : '⚙️ Agents Setup ▾';
  }
}

function syncAgentModels(val) {
  // Model inheritance handled dynamically during turn run
}

function loadChannelTemplate(type) {
  const a1 = document.getElementById('agent1-name');
  const p1 = document.getElementById('agent1-prompt');
  const a2 = document.getElementById('agent2-name');
  const p2 = document.getElementById('agent2-prompt');
  const a3 = document.getElementById('agent3-name');
  const p3 = document.getElementById('agent3-prompt');

  const sw1 = document.getElementById('agent1-enabled');
  const sw2 = document.getElementById('agent2-enabled');
  const sw3 = document.getElementById('agent3-enabled');

  if (type === 'debate') {
    if (a1 && p1) { a1.value = 'Agent 1 (Visionary)'; p1.value = 'You are Agent 1 (The Visionary & Optimist). You focus on high-impact potential, breakthrough opportunities, and progressive innovation. Present strong positive arguments and build constructively on user inquiries and other agents.'; }
    if (a2 && p2) { a2.value = 'Agent 2 (Risk Skeptic)'; p2.value = 'You are Agent 2 (The Risk Analyst & Skeptic). You question underlying assumptions, highlight edge cases, cost factors, scalability limits, and security vulnerabilities. Directly critique points raised by Agent 1 or other participants.'; }
    if (a3 && p3) { a3.value = 'Agent 3 (Pragmatist)'; p3.value = 'You are Agent 3 (The Pragmatic Synthesizer). You evaluate the optimistic perspective of Agent 1 against the risks raised by Agent 2. Offer realistic compromises, actionable roadmaps, and balanced final recommendations.'; }
    if (sw1) { sw1.checked = true; onAgentToggle(1, true); }
    if (sw2) { sw2.checked = true; onAgentToggle(2, true); }
    if (sw3) { sw3.checked = true; onAgentToggle(3, true); }
  } else if (type === '1v1') {
    if (a1 && p1) { a1.value = 'Agent 1 (Proponent)'; p1.value = 'You are Agent 1 (The Proponent). Strongly advocate for the topic, highlighting key advantages, practical benefits, and forward-looking possibilities.'; }
    if (a2 && p2) { a2.value = 'Agent 2 (Opponent)'; p2.value = 'You are Agent 2 (The Challenger). Critically examine and debate Agent 1\'s arguments, offering sharp counter-examples, alternative viewpoints, and highlighting overlooked complications.'; }
    if (sw1) { sw1.checked = true; onAgentToggle(1, true); }
    if (sw2) { sw2.checked = true; onAgentToggle(2, true); }
    if (sw3) { sw3.checked = false; onAgentToggle(3, false); } // Disable Agent 3 for 1-on-1!
  } else if (type === 'codeteam') {
    if (a1 && p1) { a1.value = 'Agent 1 (Architect)'; p1.value = 'You are Agent 1 (Principal Software Architect). Focus on clean architecture, modularity, idiomatic Go/distributed patterns, and separation of concerns. Critique and propose elegant structural designs.'; }
    if (a2 && p2) { a2.value = 'Agent 2 (Security & Perf)'; p2.value = 'You are Agent 2 (Security Auditor & Performance Engineer). Focus strictly on concurrency hazards, race conditions, memory leaks, algorithmic complexity, latency, and attack vectors in any proposed code or architecture.'; }
    if (a3 && p3) { a3.value = 'Agent 3 (QA & DevExp)'; p3.value = 'You are Agent 3 (QA Lead & Dev Experience Specialist). Focus on testability, API simplicity, error handling elegance, observability/metrics, and production maintainability.'; }
    if (sw1) { sw1.checked = true; onAgentToggle(1, true); }
    if (sw2) { sw2.checked = true; onAgentToggle(2, true); }
    if (sw3) { sw3.checked = true; onAgentToggle(3, true); }
  } else if (type === 'dialectic') {
    if (a1 && p1) { a1.value = 'Agent 1 (Σωκρατικός)'; p1.value = 'Είσαι ο Agent 1 (Σωκρατικός Ερευνητής). Εξετάζεις τα θέματα σε βάθος, θέτεις θεμελιώδη ερωτήματα, αναλύεις τις έννοιες και προκαλείς εποικοδομητικό διάλογο, απαντώντας σε άπταιστα ελληνικά.'; }
    if (a2 && p2) { a2.value = 'Agent 2 (Ρεαλιστής)'; p2.value = 'Είσαι ο Agent 2 (Κριτικός Ρεαλιστής). Εστιάζεις στην πρακτική εφαρμογή, αναδεικνύεις αντιφάσεις, δυσκολίες και ρεαλιστικούς περιορισμούς, αμφισβητώντας τις θεωρητικές υποθέσεις του Agent 1.'; }
    if (a3 && p3) { a3.value = 'Agent 3 (Συνθέτης)'; p3.value = 'Είσαι ο Agent 3 (Διαλεκτικός Συνθέτης). Συνθέτεις τη θέση του Agent 1 με την αντίθεση του Agent 2, επιλύεις τις συγκρούσεις και διατυπώνεις ένα ώριμο, ισορροπημένο και πρακτικό συμπέρασμα.'; }
    if (sw1) { sw1.checked = true; onAgentToggle(1, true); }
    if (sw2) { sw2.checked = true; onAgentToggle(2, true); }
    if (sw3) { sw3.checked = true; onAgentToggle(3, true); }
  }
  updateActiveAgentsBadge();
  updateChannelRoundInfo();
}

function getRoundAgentOrder(activeAgents, orderMode) {
  if (!activeAgents || activeAgents.length <= 1) {
    return [0];
  }

  if (orderMode === 'forward') {
    return activeAgents.map((_, idx) => idx);
  } else if (orderMode === 'reverse') {
    return activeAgents.map((_, idx) => idx).reverse();
  } else if (orderMode === 'random') {
    const allIndices = activeAgents.map((_, idx) => idx);
    let eligible = allIndices;
    if (lastFirstAgentId !== null) {
      const filtered = allIndices.filter(idx => activeAgents[idx].id !== lastFirstAgentId);
      if (filtered.length > 0) {
        eligible = filtered;
      }
    }
    const firstChoice = eligible[Math.floor(Math.random() * eligible.length)];
    lastFirstAgentId = activeAgents[firstChoice].id; // Track first agent ID

    const remaining = allIndices.filter(idx => idx !== firstChoice);
    for (let i = remaining.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [remaining[i], remaining[j]] = [remaining[j], remaining[i]];
    }
    return [firstChoice, ...remaining];
  }
  return activeAgents.map((_, idx) => idx);
}

async function runChannelDiscussion() {
  if (isChannelRunning) return;

  const userInputEl = document.getElementById('channel-user-input');
  const userText = userInputEl.value.trim();

  if (!userText && channelHistory.length === 0) {
    alert('Please write an initial message or topic to start the discussion!');
    userInputEl.focus();
    return;
  }

  const baseModel = document.getElementById('channel-model').value;
  if (!baseModel) {
    alert('Please select a Channel Model from the toolbar first!');
    return;
  }

  const activeAgents = getEnabledAgents();
  if (activeAgents.length === 0) {
    alert('Please enable at least one agent in the setup panel to start the discussion!');
    return;
  }

  const rounds = parseInt(document.getElementById('channel-rounds').value, 10) || 1;
  const orderMode = document.getElementById('channel-order').value || 'forward';
  const think = document.getElementById('channel-think-toggle').checked;
  const keepAlive = document.getElementById('channel-keepalive').value || '1h';

  const chatFeed = document.getElementById('channel-chat-feed');
  const emptyState = chatFeed.querySelector('.channel-empty-state');
  if (emptyState) {
    emptyState.remove();
  }

  // If user entered new text, add user message
  if (userText) {
    channelHistory.push({
      sender: 'user',
      name: 'User',
      content: userText,
    });

    const userCard = document.createElement('div');
    userCard.className = 'channel-msg-card user-card';
    const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    userCard.innerHTML = `
      <div class="channel-msg-header">
        <div class="msg-header-left">
          <span class="speaker-pill pill-user">👤 You</span>
          <span class="speaker-name">User</span>
        </div>
        <div class="msg-header-right">
          <span class="speaker-metrics">${timeStr}</span>
        </div>
      </div>
      <div class="channel-msg-body">${escapeHtml(userText)}</div>
    `;
    chatFeed.appendChild(userCard);
    userInputEl.value = '';
    chatFeed.scrollTop = chatFeed.scrollHeight;
  }

  isChannelRunning = true;
  document.getElementById('channel-run-btn').classList.add('hidden');
  document.getElementById('channel-stop-btn').classList.remove('hidden');
  const statusPill = document.getElementById('channel-status-pill');
  statusPill.className = 'status-pill running';

  try {
    for (let r = 1; r <= rounds; r++) {
      if (!isChannelRunning) break;

      const agentOrder = getRoundAgentOrder(activeAgents, orderMode);

      // Round Divider
      const divider = document.createElement('div');
      divider.className = 'channel-round-divider';
      const orderLabels = agentOrder.map(idx => activeAgents[idx].name).join(' ➔ ');
      divider.innerHTML = `<span class="round-tag">Round ${r} of ${rounds} • Turn Order: ${orderLabels}</span>`;
      chatFeed.appendChild(divider);
      chatFeed.scrollTop = chatFeed.scrollHeight;

      // Execute each active agent in the calculated order
      for (let t = 0; t < agentOrder.length; t++) {
        if (!isChannelRunning) break;

        const agentIdx = agentOrder[t];
        const agent = activeAgents[agentIdx];

        statusPill.textContent = `Round ${r}/${rounds} • Turn ${t + 1}/${activeAgents.length}: ${agent.name} speaking...`;

        // Create Agent Message Card
        const card = document.createElement('div');
        card.className = `channel-msg-card ${agent.cardClass}`;
        const turnMsgId = 'channel-msg-' + Date.now() + '-' + agent.id;
        card.innerHTML = `
          <div class="channel-msg-header">
            <div class="msg-header-left">
              <span class="speaker-pill ${agent.pillClass}">${agent.id}</span>
              <span class="speaker-name">${escapeHtml(agent.name)}</span>
              <span class="model-tag">${agent.model}</span>
            </div>
            <div class="msg-header-right">
              <span class="speaker-metrics" id="${turnMsgId}-metrics">Streaming...</span>
              <button class="btn btn-xs btn-outline" onclick="copyCardText('${turnMsgId}-body')" title="Copy message">📋</button>
            </div>
          </div>
          <div class="channel-msg-body" id="${turnMsgId}-body"></div>
        `;
        chatFeed.appendChild(card);
        chatFeed.scrollTop = chatFeed.scrollHeight;

        const cardBody = document.getElementById(`${turnMsgId}-body`);
        const metricsEl = document.getElementById(`${turnMsgId}-metrics`);
        const renderer = new StreamRenderer(cardBody, false);

        // Build History for this agent
        const historyPayload = [];
        for (const item of channelHistory) {
          if (item.sender === 'user') {
            historyPayload.push({
              role: 'user',
              content: `${item.name}: ${item.content}`,
            });
          } else if (item.sender === 'agent') {
            if (item.agentId === agent.id) {
              historyPayload.push({
                role: 'assistant',
                content: item.content,
              });
            } else {
              historyPayload.push({
                role: 'user',
                content: `[${item.name}]: ${item.content}`,
              });
            }
          }
        }

        const controller = new AbortController();
        channelAbortController = controller;
        let assistantReply = '';

        try {
          const response = await fetch('/api/chat', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              model: agent.model,
              system_prompt: agent.prompt + '\n\n(You are participating in a multi-agent discussion channel with the user and other agents. Address the discussion naturally, building upon, analyzing, or respectfully challenging the previous responses.)',
              user_prompt: '',
              history: historyPayload,
              think: think,
              keep_alive: keepAlive,
            }),
            signal: controller.signal,
          });

          if (!response.ok) {
            throw new Error(`API error (${response.status})`);
          }

          const reader = response.body.getReader();
          const decoder = new TextDecoder('utf-8');
          let buffer = '';

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n\n');
            buffer = lines.pop();

            for (const line of lines) {
              if (!line.startsWith('data: ')) continue;
              try {
                const chunk = JSON.parse(line.slice(6));
                if (chunk.error) {
                  renderer.error(chunk.error);
                } else {
                  renderer.append(chunk.delta, chunk.thinking);
                  assistantReply += chunk.delta || '';
                }

                if (chunk.done && chunk.metrics) {
                  renderer.finish(chunk.metrics.total_duration_ms / 1000);
                  if (metricsEl) {
                    metricsEl.textContent = `${chunk.metrics.output_tokens_per_s.toFixed(1)} tok/s • ${chunk.metrics.output_tokens} tok • ${(chunk.metrics.total_duration_ms / 1000).toFixed(1)}s`;
                  }
                }
              } catch (e) {}
            }
            const isNearBottom = (chatFeed.scrollHeight - chatFeed.scrollTop - chatFeed.clientHeight) < 160;
            if (isNearBottom) {
              chatFeed.scrollTop = chatFeed.scrollHeight;
            }
          }

          if (!controller.signal.aborted && assistantReply.trim()) {
            channelHistory.push({
              sender: 'agent',
              agentId: agent.id,
              name: agent.name,
              content: assistantReply,
            });
          }
        } catch (err) {
          if (err.name === 'AbortError') {
            renderer.append('\n\n[⏹ Stopped]');
            if (metricsEl) metricsEl.textContent = 'Stopped';
          } else {
            renderer.error(err.message);
            if (metricsEl) metricsEl.textContent = 'Error';
          }
        } finally {
          channelAbortController = null;
        }

        const isNearBottomEnd = (chatFeed.scrollHeight - chatFeed.scrollTop - chatFeed.clientHeight) < 180;
        if (isNearBottomEnd) {
          chatFeed.scrollTop = chatFeed.scrollHeight;
        }
        // Pause briefly between agents
        if (isChannelRunning) {
          await new Promise(res => setTimeout(res, 350));
        }
      }
    }
  } finally {
    if (!isChannelRunning) {
      statusPill.className = 'status-pill idle';
      statusPill.textContent = '⏹ Discussion paused';
    } else {
      statusPill.className = 'status-pill completed';
      statusPill.textContent = `✅ Completed (${rounds} round${rounds > 1 ? 's' : ''}) • Ready for your reply`;
    }

    isChannelRunning = false;
    document.getElementById('channel-run-btn').classList.remove('hidden');
    document.getElementById('channel-stop-btn').classList.add('hidden');
    userInputEl.focus();
  }
}

function stopChannel() {
  if (channelAbortController) {
    channelAbortController.abort();
    channelAbortController = null;
  }
  isChannelRunning = false;
  const statusPill = document.getElementById('channel-status-pill');
  if (statusPill) {
    statusPill.className = 'status-pill idle';
    statusPill.textContent = '⏹ Stopping...';
  }
}

function clearChannel() {
  if (isChannelRunning) {
    stopChannel();
  }
  channelHistory = [];
  lastFirstAgentId = null;
  const chatFeed = document.getElementById('channel-chat-feed');
  chatFeed.innerHTML = `
    <div class="channel-empty-state">
      <span class="empty-icon">🎙️</span>
      <h3>Channel Multi-Agent Playground</h3>
      <p>Type a topic or question below. The active agents will take turns discussing, analyzing, and building upon each other's ideas in real time.</p>
    </div>
  `;
  const statusPill = document.getElementById('channel-status-pill');
  if (statusPill) {
    statusPill.className = 'status-pill idle';
    statusPill.textContent = 'Idle';
  }
}

function copyCardText(elementId) {
  const el = document.getElementById(elementId);
  if (!el) return;
  const text = el.innerText || el.textContent;
  if (text) {
    navigator.clipboard.writeText(text.trim());
    alert('Copied message to clipboard!');
  }
}

function copyChannelTranscript() {
  if (!channelHistory || channelHistory.length === 0) {
    alert('No discussion messages in the channel to copy!');
    return;
  }
  let transcript = '# 🎙️ Ollama Lab - Channel Discussion\n\n';
  for (const item of channelHistory) {
    if (item.sender === 'user') {
      transcript += `### 👤 ${item.name}:\n${item.content}\n\n---\n\n`;
    } else {
      transcript += `### 🤖 ${item.name}:\n${item.content}\n\n---\n\n`;
    }
  }
  navigator.clipboard.writeText(transcript);
  alert('Full channel transcript copied to clipboard as Markdown!');
}

function setupChannelFeedScroll() {
  const feed = document.getElementById('channel-chat-feed');
  const btn = document.getElementById('channel-scroll-bottom-btn');
  if (!feed || !btn) return;

  feed.addEventListener('scroll', () => {
    const isScrolledUp = (feed.scrollHeight - feed.scrollTop - feed.clientHeight) > 160;
    if (isScrolledUp) {
      btn.classList.remove('hidden');
    } else {
      btn.classList.add('hidden');
    }
  });
}

function scrollChannelToBottom() {
  const feed = document.getElementById('channel-chat-feed');
  if (feed) {
    feed.scrollTo({ top: feed.scrollHeight, behavior: 'smooth' });
  }
}


