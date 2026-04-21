<script>
  import { onMount, tick } from 'svelte';
  import QRCode from 'qrcode';
  import { api } from '../lib/api.js';

  // ── Tab ────────────────────────────────────────────────────────────────────
  let activeTab = 'server'; // 'server' | 'browser' | 'debug'

  // ── Browser Flash ──────────────────────────────────────────────────────────
  import 'esp-web-tools';

  let browserFlashState = 'idle'; // idle | connecting | writing | done | error
  let browserFlashMsg = '';
  let firmwareAvailable = false;
  let latestVersion = '';

  // Browser flash wizard — 5 steps mirroring Server Flash
  let bfStep = 1;      // 1=template 2=name 3=wifi 4=flash 5=done
  let bfTemplate = null;
  let bfDeviceName = '';
  let bfSSID = '';
  let bfPassword = '';
  let bfToken = '';
  let bfFW = '';
  let bfFlashing = false;
  let bfFlashError = '';
  let bfPairing = null; // { discriminator, passcode, qr_payload } | null
  let bfQrDataUrl = '';

  async function bfDoFlash() {
    bfFlashError = '';
    bfFlashing = true;
    try {
      const res = await fetch('/api/webflash/prepare', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          template_id:   bfTemplate.id,
          device_name:   bfDeviceName,
          wifi_ssid:     bfSSID,
          wifi_password: bfPassword,
          fw_version:    bfFW,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      bfToken = data.token;
      bfPairing = { discriminator: data.discriminator, passcode: data.passcode, qr_payload: data.qr_payload };
      bfQrDataUrl = await QRCode.toDataURL(data.qr_payload, { width: 180, margin: 2 });
      browserFlashState = 'idle';
      browserFlashMsg = '';
    } catch (e) {
      bfFlashError = e.message;
    } finally {
      bfFlashing = false;
    }
  }

  // Called by esp-web-tools once flashing finishes or errors
  function handleInstallEvent(e) {
    const state = e.detail?.state;
    if (!state) return;
    if (state === 'finished') {
      browserFlashState = 'done';
      bfStep = 5;
    } else if (state === 'error') {
      browserFlashState = 'error';
      browserFlashMsg = e.detail?.message || 'Flash failed';
    } else if (state === 'initializing' || state === 'preparing') {
      browserFlashState = 'connecting';
      browserFlashMsg = 'Connecting to device…';
    } else if (state === 'writing') {
      browserFlashState = 'writing';
      browserFlashMsg = e.detail?.details || 'Writing firmware…';
    }
  }

  function bfReset() {
    bfStep = 1; bfTemplate = null; bfDeviceName = ''; bfSSID = ''; bfPassword = '';
    bfFW = latestVersion; bfToken = ''; bfFlashError = ''; browserFlashState = 'idle'; browserFlashMsg = '';
    bfPairing = null; bfQrDataUrl = '';
  }

  // ── Browser Flash — ESPHome path ───────────────────────────────────────────
  let bfFirmwareType = 'matter'; // 'matter' | 'esphome'
  let bfEspStep = 1;             // 1=board 2=components 3=config 4=compile 5=flash 6=done
  let bfEspBoard = 'esp32-c3';
  let bfEspComponents = [];
  let bfEspDeviceName = '';
  let bfEspWifiSSID = '';
  let bfEspWifiPassword = '';
  let bfEspHubURL = window.location.origin;
  let bfEspHA = false;
  let bfEspCompiling = false;
  let bfEspError = '';
  let bfEspLogs = [];
  let bfEspToken = '';
  let bfEspFlashState = 'idle'; // idle | connecting | writing | done | error
  let bfEspFlashMsg = '';
  let bfEspModules = [];
  let bfEspInstallEl = null;

  async function loadBfEspModules() {
    try {
      bfEspModules = await api.get('/api/modules?esphome=true');
    } catch (e) {
      bfEspError = 'Failed to load modules: ' + e.message;
    }
  }

  function bfEspAddComponent(moduleId) {
    const mod = bfEspModules.find(m => m.id === moduleId);
    if (!mod) return;
    const pins = {};
    (mod.io || []).forEach(p => { pins[p.id] = p.default ?? ''; });
    bfEspComponents = [...bfEspComponents, { type: moduleId, name: mod.name, pins, io: mod.io || [] }];
  }

  function bfEspRemoveComponent(i) {
    bfEspComponents = bfEspComponents.filter((_, idx) => idx !== i);
  }

  async function bfEspDoCompile() {
    bfEspError = '';
    bfEspCompiling = true;
    try {
      const res = await fetch('/api/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          board:          bfEspBoard,
          components:     bfEspComponents.map(c => ({ type: c.type, name: c.name, pins: c.pins })),
          device_name:    bfEspDeviceName,
          wifi_ssid:      bfEspWifiSSID,
          wifi_password:  bfEspWifiPassword,
          ha_integration: bfEspHA,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      window.dispatchEvent(new CustomEvent('navigate', {
        detail: { view: 'jobmonitor', jobId: data.id }
      }));
    } catch (e) {
      bfEspError = e.message;
    } finally {
      bfEspCompiling = false;
    }
  }

  function handleBfEspInstallEvent(e) {
    const state = e.detail?.state;
    if (!state) return;
    if (state === 'finished') { bfEspFlashState = 'done'; bfEspStep = 6; }
    else if (state === 'error') { bfEspFlashState = 'error'; bfEspFlashMsg = e.detail?.message || 'Flash failed'; }
    else if (state === 'initializing' || state === 'preparing') { bfEspFlashState = 'connecting'; bfEspFlashMsg = 'Connecting…'; }
    else if (state === 'writing') { bfEspFlashState = 'writing'; bfEspFlashMsg = e.detail?.details || 'Writing…'; }
  }

  $: if (bfEspInstallEl) {
    bfEspInstallEl.removeEventListener('state-changed', handleBfEspInstallEvent);
    bfEspInstallEl.addEventListener('state-changed', handleBfEspInstallEvent);
  }

  function bfEspReset() {
    bfFirmwareType = 'matter';
    bfEspStep = 1; bfEspBoard = 'esp32-c3'; bfEspComponents = [];
    bfEspDeviceName = ''; bfEspWifiSSID = ''; bfEspWifiPassword = '';
    bfEspHA = false; bfEspCompiling = false; bfEspError = '';
    bfEspLogs = []; bfEspToken = ''; bfEspFlashState = 'idle'; bfEspFlashMsg = '';
  }

  // ── Server Flash (existing wizard) ────────────────────────────────────────
  let step = 1;
  let templates = [];
  let firmware = [];
  let ports = [];
  let loadingInit = true;
  let error = '';

  let selectedTemplate = null;
  let deviceNames = [''];
  let wifiSSID = '';
  let wifiPassword = '';
  let selectedPort = '';
  let selectedFW = '';

  let flashing = false;
  let results = [];
  let flashError = '';

  onMount(async () => {
    try {
      [templates, firmware, ports] = await Promise.all([
        api.get('/api/templates'),
        api.get('/api/firmware'),
        api.get('/api/flash/ports'),
      ]);
      const latest = firmware.find(f => f.is_latest);
      if (latest) {
        selectedFW = latest.version;
        bfFW = latest.version;
        firmwareAvailable = true;
        latestVersion = latest.version;
      }
    } catch (e) {
      error = e.message;
    } finally {
      loadingInit = false;
    }
  });

  async function refreshPorts() {
    ports = await api.get('/api/flash/ports');
  }

  function addName() { deviceNames = [...deviceNames, '']; }
  function removeName(i) { deviceNames = deviceNames.filter((_, idx) => idx !== i); }

  async function doFlash() {
    flashError = '';
    flashing = true;
    results = [];
    try {
      results = await api.post('/api/flash/run', {
        template_id:   selectedTemplate.id,
        device_names:  deviceNames.filter(n => n.trim()),
        wifi_ssid:     wifiSSID,
        wifi_password: wifiPassword,
        port:          selectedPort,
        fw_version:    selectedFW,
      });
      step = 5;
    } catch (e) {
      flashError = e.message;
    } finally {
      flashing = false;
    }
  }

  function reset() {
    step = 1; selectedTemplate = null; deviceNames = [''];
    wifiSSID = ''; wifiPassword = ''; results = []; flashError = '';
    selectedPort = ''; selectedFW = latestVersion;
  }

  // ── ESPHome flash path ─────────────────────────────────────────────────────
  let firmwareType = 'matter'; // 'matter' | 'esphome'
  let espStep = 1;             // 1=board 2=components 3=config 4=flash 6=done
  let espBoard = 'esp32-c3';
  let espComponents = [];      // [{type, name, pins: {ROLE: 'GPIO4'}}]
  let espDeviceName = '';
  let espWifiSSID = '';
  let espWifiPassword = '';
  let espHubURL = window.location.origin;
  let espHA = false;
  let espFlashing = false;
  let espFlashError = '';
  let espLogs = [];
  let espResult = null;        // {ok, device_id, name, error} | null
  let espModules = [];         // loaded from /api/modules?esphome=true
  let espInstallEl = null;

  async function loadESPHomeModules() {
    try {
      espModules = await api.get('/api/modules?esphome=true');
    } catch (e) {
      espFlashError = 'Failed to load modules: ' + e.message;
    }
  }

  function espAddComponent(moduleId) {
    const mod = espModules.find(m => m.id === moduleId);
    if (!mod) return;
    const pins = {};
    (mod.io || []).forEach(p => { pins[p.id] = p.default ?? ''; });
    espComponents = [...espComponents, { type: moduleId, name: mod.name, pins, io: mod.io || [] }];
  }

  function espRemoveComponent(i) {
    espComponents = espComponents.filter((_, idx) => idx !== i);
  }

  async function espDoFlash() {
    espFlashError = '';
    espFlashing = true;
    espLogs = [];
    espResult = null;
    try {
      const res = await fetch('/api/flash/esphome', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          port:           selectedPort,
          device_name:    espDeviceName,
          wifi_ssid:      espWifiSSID,
          wifi_password:  espWifiPassword,
          hub_url:        espHubURL,
          board:          espBoard,
          ha_integration: espHA,
          components:     espComponents,
        }),
      });
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop();
        for (const line of lines) {
          if (!line.trim()) continue;
          try {
            const obj = JSON.parse(line);
            if (obj.log !== undefined) espLogs = [...espLogs, obj.log];
            else espResult = obj;
          } catch {}
        }
      }
      espStep = 6;
    } catch (e) {
      espFlashError = e.message;
    } finally {
      espFlashing = false;
    }
  }

  function espReset() {
    espStep = 1; espBoard = 'esp32-c3'; espComponents = [];
    espDeviceName = ''; espWifiSSID = ''; espWifiPassword = '';
    espHA = false; espFlashing = false; espFlashError = '';
    espLogs = []; espResult = null; firmwareType = 'matter';
  }

  // ── GPIO lists per board ──────────────────────────────────────────────────
  const BOARD_GPIOS = {
    'esp32-c3':  ['GPIO0','GPIO1','GPIO2','GPIO3','GPIO4','GPIO5','GPIO6','GPIO7',
                  'GPIO8','GPIO9','GPIO10','GPIO18','GPIO19','GPIO20','GPIO21'],
    'esp32-h2':  ['GPIO0','GPIO1','GPIO2','GPIO3','GPIO4','GPIO5','GPIO6','GPIO7',
                  'GPIO8','GPIO9','GPIO10','GPIO11','GPIO12','GPIO13','GPIO14','GPIO15',
                  'GPIO16','GPIO17','GPIO18','GPIO19','GPIO20','GPIO21','GPIO22','GPIO23',
                  'GPIO25','GPIO26','GPIO27'],
    'esp32':     ['GPIO0','GPIO1','GPIO2','GPIO3','GPIO4','GPIO5',
                  'GPIO12','GPIO13','GPIO14','GPIO15','GPIO16','GPIO17',
                  'GPIO18','GPIO19','GPIO21','GPIO22','GPIO23',
                  'GPIO25','GPIO26','GPIO27',
                  'GPIO32','GPIO33','GPIO34','GPIO35','GPIO36','GPIO39'],
    'esp32-s3':  ['GPIO1','GPIO2','GPIO3','GPIO4','GPIO5','GPIO6','GPIO7',
                  'GPIO8','GPIO9','GPIO10','GPIO11','GPIO12','GPIO13','GPIO14',
                  'GPIO15','GPIO16','GPIO17','GPIO18','GPIO19','GPIO20','GPIO21',
                  'GPIO35','GPIO36','GPIO37','GPIO38','GPIO39','GPIO40',
                  'GPIO41','GPIO42','GPIO43','GPIO44','GPIO45','GPIO46','GPIO47','GPIO48'],
  };
  function boardGpios(board) { return BOARD_GPIOS[board] ?? BOARD_GPIOS['esp32-c3']; }

  // ── Serial Debug ───────────────────────────────────────────────────────────
  const BAUD_RATES = [74880, 115200, 921600];
  const LOG_MAX_LINES = 1000;

  let dbgStatus = 'disconnected'; // 'disconnected' | 'connecting' | 'connected' | 'error'
  let dbgError  = '';
  let dbgPort   = null;
  let dbgAbort  = null;
  let dbgPipeDone = null;
  let logLines  = [];
  let selectedBaud = 115200;
  let autoScroll = true;
  let inputLine = '';
  let logContainer;
  let serialSupported = typeof navigator !== 'undefined' && 'serial' in navigator;

  const STATUS_LABEL = { disconnected: 'Disconnected', connecting: 'Connecting…', connected: 'Connected', error: 'Error' };
  const STATUS_BADGE = { disconnected: 'badge-ghost', connecting: 'badge-warning', connected: 'badge-success', error: 'badge-error' };

  class LineBreakTransformer {
    constructor() { this.buffer = ''; }
    transform(chunk, controller) {
      this.buffer += chunk;
      const lines = this.buffer.split(/\r?\n/);
      this.buffer = lines.pop();
      for (const line of lines) controller.enqueue(line);
    }
    flush(controller) {
      if (this.buffer) controller.enqueue(this.buffer);
    }
  }

  async function appendLines(newLines) {
    logLines = [...logLines, ...newLines].slice(-LOG_MAX_LINES);
    if (autoScroll) {
      await tick();
      if (logContainer) logContainer.scrollTop = logContainer.scrollHeight;
    }
  }

  async function connect() {
    dbgError = '';
    dbgStatus = 'connecting';
    try {
      dbgPort = await navigator.serial.requestPort();
    } catch (_) {
      dbgStatus = 'disconnected'; // user cancelled picker
      return;
    }
    try {
      await dbgPort.open({ baudRate: selectedBaud });
      dbgStatus = 'connected';
      startReading();
    } catch (e) {
      dbgStatus = 'error';
      dbgError = e.message;
      dbgPort = null;
    }
  }

  function startReading() {
    dbgAbort = new AbortController();
    dbgPipeDone = dbgPort.readable
      .pipeThrough(new TextDecoderStream(), { signal: dbgAbort.signal })
      .pipeThrough(new TransformStream(new LineBreakTransformer()))
      .pipeTo(new WritableStream({
        write(line) { return appendLines([line]); }
      }))
      .catch(async e => {
        if (e.name !== 'AbortError') {
          dbgError = e.message;
          dbgStatus = 'error';
          try { await dbgPort?.close(); } catch (_) {}
          dbgPort = null;
        }
      });
  }

  async function disconnect() {
    if (dbgAbort) { dbgAbort.abort(); dbgAbort = null; }
    if (dbgPipeDone) { await dbgPipeDone; dbgPipeDone = null; }
    try { await dbgPort?.close(); } catch (_) {}
    dbgPort = null;
    dbgStatus = 'disconnected';
  }

  async function sendLine() {
    if (!dbgPort || dbgStatus !== 'connected' || !inputLine.trim()) return;
    const encoder = new TextEncoder();
    let writer;
    try {
      writer = dbgPort.writable.getWriter();
      await writer.write(encoder.encode(inputLine + '\r\n'));
    } catch (e) {
      dbgError = e.message;
    } finally {
      try { writer?.releaseLock(); } catch (_) {}
    }
    appendLines(['> ' + inputLine]);
    inputLine = '';
  }

  function clearLog() { logLines = []; }
</script>

<div class="p-6 flex flex-col gap-4 max-w-2xl">
  <h2 class="text-lg font-semibold">Flash Devices</h2>

  <!-- Tab switcher -->
  <div role="tablist" class="tabs tabs-bordered">
    <button role="tab" class="tab {activeTab === 'server' ? 'tab-active' : ''}"
      on:click={() => activeTab = 'server'}>
      Server Flash <span class="ml-1 text-xs text-base-content/40">(ESP32 on RPi USB)</span>
    </button>
    <button role="tab" class="tab {activeTab === 'browser' ? 'tab-active' : ''}"
      on:click={() => activeTab = 'browser'}>
      Browser Flash <span class="ml-1 text-xs text-base-content/40">(ESP32 on your USB)</span>
    </button>
    <button role="tab" class="tab {activeTab === 'debug' ? 'tab-active' : ''}"
      on:click={() => activeTab = 'debug'}>
      Serial Debug <span class="ml-1 text-xs text-base-content/40">(USB logs)</span>
    </button>
  </div>

  <!-- ── Browser Flash ──────────────────────────────────────────────────── -->
  {#if activeTab === 'browser'}
    {#if loadingInit}
      <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
    {:else if error}
      <div class="alert alert-error text-sm">{error}</div>
    {:else}

    <!-- Firmware type toggle -->
    <div class="flex gap-2">
      <button class="btn btn-sm {bfFirmwareType === 'matter' ? 'btn-primary' : 'btn-ghost'}"
        on:click={() => { bfFirmwareType = 'matter'; }}>Matter</button>
      <button class="btn btn-sm {bfFirmwareType === 'esphome' ? 'btn-primary' : 'btn-ghost'}"
        on:click={() => { bfFirmwareType = 'esphome'; loadBfEspModules(); }}>ESPHome</button>
    </div>

    {#if bfFirmwareType === 'matter'}
    <ul class="steps steps-horizontal w-full text-xs">
      <li class="step {bfStep >= 1 ? 'step-primary' : ''}">Template</li>
      <li class="step {bfStep >= 2 ? 'step-primary' : ''}">Name</li>
      <li class="step {bfStep >= 3 ? 'step-primary' : ''}">WiFi</li>
      <li class="step {bfStep >= 4 ? 'step-primary' : ''}">Flash</li>
      <li class="step {bfStep >= 5 ? 'step-primary' : ''}">Done</li>
    </ul>

    {#if bfStep === 1}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">Select a template</div>
        {#if templates.length === 0}
          <div class="text-sm text-base-content/50">No templates yet — create one in the Templates view.</div>
        {:else}
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {#each templates as t}
              <button
                class="card p-3 border text-left transition-all
                  {bfTemplate?.id === t.id ? 'border-primary bg-primary/10' : 'border-base-300 bg-base-200 hover:border-primary/40'}"
                on:click={() => bfTemplate = t}
              >
                <div class="font-semibold text-sm">{t.name || t.id}</div>
                <div class="text-xs text-base-content/50">{t.board}</div>
              </button>
            {/each}
          </div>
          <button class="btn btn-primary btn-sm self-end" disabled={!bfTemplate}
            on:click={() => bfStep = 2}>Next →</button>
        {/if}
      </div>

    {:else if bfStep === 2}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">Device name <span class="text-base-content/40 font-normal">(e.g. 1/Bedroom)</span></div>
        <input class="input input-bordered input-sm" placeholder="e.g. 1/Bedroom"
          bind:value={bfDeviceName} />
        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm" on:click={() => bfStep = 1}>← Back</button>
          <button class="btn btn-primary btn-sm"
            disabled={!bfDeviceName.trim()}
            on:click={() => bfStep = 3}>Next →</button>
        </div>
      </div>

    {:else if bfStep === 3}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">WiFi credentials <span class="text-base-content/40 font-normal">(optional)</span></div>
        <input class="input input-bordered input-sm" placeholder="WiFi SSID" bind:value={bfSSID} />
        <input class="input input-bordered input-sm" type="password" placeholder="WiFi password" bind:value={bfPassword} />

        <div class="divider my-1"></div>
        <div class="text-sm font-semibold">Firmware version</div>
        {#if firmware.length === 0}
          <div class="text-sm text-base-content/50">No firmware uploaded — go to the Firmware view first.</div>
        {:else}
          <select class="select select-bordered select-sm" bind:value={bfFW}>
            <option value="">Select version…</option>
            {#each firmware as f}<option value={f.version}>{f.version}{f.is_latest ? ' (latest)' : ''}</option>{/each}
          </select>
        {/if}

        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm" on:click={() => bfStep = 2}>← Back</button>
          <button class="btn btn-primary btn-sm"
            disabled={!bfFW}
            on:click={() => bfStep = 4}>Next →</button>
        </div>
      </div>

    {:else if bfStep === 4}
      <div class="flex flex-col gap-3">
        <div class="card bg-base-200 border border-base-300 p-4 text-sm space-y-1">
          <div><strong>Template:</strong> {bfTemplate.name || bfTemplate.id}</div>
          <div><strong>Device:</strong> {bfDeviceName}</div>
          <div><strong>WiFi:</strong> {bfSSID || '— (none)'}</div>
          <div><strong>Firmware:</strong> {bfFW}</div>
          <div class="text-xs text-base-content/50 pt-1">
            Plug your ESP32-C3 into <strong>this computer</strong> via USB, then click Flash Now.
            <br>Requires Chrome or Edge (Web Serial API).
          </div>
        </div>
        {#if bfFlashError}<div class="alert alert-error text-sm">{bfFlashError}</div>{/if}
        {#if browserFlashState === 'error'}
          <div class="alert alert-error text-sm">{browserFlashMsg || 'Flash failed'}</div>
        {/if}
        {#if browserFlashState !== 'idle' && browserFlashState !== 'error'}
          <div class="flex items-center gap-2 text-sm text-base-content/70">
            <span class="loading loading-spinner loading-xs"></span>
            {browserFlashMsg}
          </div>
        {/if}
        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm"
            disabled={bfFlashing || browserFlashState !== 'idle'}
            on:click={() => { bfStep = 3; bfToken = ''; browserFlashState = 'idle'; bfPairing = null; bfQrDataUrl = ''; }}>← Back</button>

          {#if !bfToken}
            <button class="btn btn-warning btn-sm" disabled={bfFlashing} on:click={bfDoFlash}>
              {#if bfFlashing}<span class="loading loading-spinner loading-xs"></span> Preparing…{:else}⚡ Flash Now{/if}
            </button>
          {:else}
            <esp-web-install-button
              manifest="/api/webflash/manifest?token={bfToken}"
              on:state-changed={handleInstallEvent}
            >
              <button slot="activate" class="btn btn-warning btn-sm"
                disabled={browserFlashState !== 'idle'}>
                {#if browserFlashState !== 'idle'}
                  <span class="loading loading-spinner loading-xs"></span> Flashing…
                {:else}
                  ⚡ Connect &amp; Flash
                {/if}
              </button>
              <span slot="unsupported" class="alert alert-error text-sm">
                Web Serial not supported — use Chrome or Edge.
              </span>
            </esp-web-install-button>
          {/if}
        </div>
      </div>

    {:else if bfStep === 5}
      <div class="flex flex-col gap-3">
        <div class="flex items-center gap-3 p-3 rounded-lg border border-success/40 bg-success/10">
          <span class="text-xl">✓</span>
          <div class="flex-1">
            <div class="font-semibold text-sm">{bfDeviceName}</div>
            <div class="text-xs text-base-content/50">Flash complete — device rebooting with WiFi + Matter config.</div>
          </div>
        </div>
        {#if bfPairing}
          <div class="flex flex-col gap-3 p-4 rounded-lg border border-base-300 bg-base-200">
            <div class="font-semibold text-sm">Commission this device</div>
            <p class="text-xs text-base-content/60">Scan with Apple Home or Google Home. Unplug and replug the device first to enter commissioning mode.</p>
            {#if bfQrDataUrl}
              <img src={bfQrDataUrl} alt="Matter QR code" class="rounded border border-base-300 self-center" width="180" />
            {/if}
            <div class="font-mono text-xs space-y-1">
              <div><span class="text-base-content/50">Discriminator:</span> {bfPairing.discriminator}</div>
              <div><span class="text-base-content/50">Passcode:</span> {bfPairing.passcode}</div>
              <div class="break-all"><span class="text-base-content/50">QR payload:</span> {bfPairing.qr_payload}</div>
            </div>
          </div>
        {/if}
        <div class="alert alert-info text-xs">
          Unplug and replug, then open the
          <button class="link link-primary font-semibold" on:click={() => activeTab = 'debug'}>Serial Debug</button>
          tab to view boot logs.
        </div>
        <button class="btn btn-ghost btn-sm self-start mt-2" on:click={bfReset}>Flash another device</button>
      </div>
    {/if}
    {/if}

    <!-- ESPHome browser flash wizard -->
    {#if bfFirmwareType === 'esphome'}
      <ul class="steps steps-horizontal w-full text-xs">
        <li class="step {bfEspStep >= 1 ? 'step-primary' : ''}">Board</li>
        <li class="step {bfEspStep >= 2 ? 'step-primary' : ''}">Components</li>
        <li class="step {bfEspStep >= 3 ? 'step-primary' : ''}">Config</li>
        <li class="step {bfEspStep >= 4 ? 'step-primary' : ''}">Compile</li>
        <li class="step {bfEspStep >= 5 ? 'step-primary' : ''}">Flash</li>
        <li class="step {bfEspStep >= 6 ? 'step-primary' : ''}">Done</li>
      </ul>

      {#if bfEspStep === 1}
        <div class="flex flex-col gap-3">
          <div class="text-sm font-semibold">Select board</div>
          {#each ['esp32-c3', 'esp32-h2', 'esp32', 'esp32-s3'] as board}
            <label class="flex items-center gap-2 cursor-pointer">
              <input type="radio" class="radio radio-sm" bind:group={bfEspBoard} value={board} />
              <span class="text-sm font-mono">{board}</span>
            </label>
          {/each}
          <div class="flex gap-2 mt-2">
            <button class="btn btn-primary btn-sm" on:click={() => bfEspStep = 2}>Next</button>
          </div>
        </div>

      {:else if bfEspStep === 2}
        <div class="flex flex-col gap-3">
          <div class="text-sm font-semibold">Add components</div>
          {#each bfEspComponents as comp, i}
            <div class="border border-base-300 rounded p-3 flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-semibold">{comp.type}</span>
                <button class="btn btn-ghost btn-xs" on:click={() => bfEspRemoveComponent(i)}>✕</button>
              </div>
              <input class="input input-bordered input-sm w-full" placeholder="Name (e.g. Room Temp)"
                bind:value={comp.name} />
              {#each Object.keys(comp.pins) as role}
                {@const pinDef = (comp.io || []).find(p => p.id === role)}
                <label class="text-xs flex items-center gap-2">
                  <span class="w-24 font-mono">{role}</span>
                  {#if pinDef?.type === 'config'}
                    <input class="input input-bordered input-xs flex-1" type="text"
                      placeholder="integer" bind:value={comp.pins[role]} />
                  {:else}
                    <select class="select select-bordered select-xs flex-1" bind:value={comp.pins[role]}>
                      <option value="">Select GPIO…</option>
                      {#each boardGpios(bfEspBoard) as gpio}
                        <option value={gpio}>{gpio}</option>
                      {/each}
                    </select>
                  {/if}
                </label>
              {/each}
            </div>
          {/each}
          <select class="select select-bordered select-sm" on:change={e => { bfEspAddComponent(e.target.value); e.target.value = ''; }}>
            <option value="">+ Add component…</option>
            {#each bfEspModules as m}
              <option value={m.id}>{m.name}</option>
            {/each}
          </select>
          <div class="flex gap-2 mt-2">
            <button class="btn btn-ghost btn-sm" on:click={() => bfEspStep = 1}>Back</button>
            <button class="btn btn-primary btn-sm" disabled={bfEspComponents.length === 0}
              on:click={() => bfEspStep = 3}>Next</button>
          </div>
        </div>

      {:else if bfEspStep === 3}
        <div class="flex flex-col gap-3">
          <div class="text-sm font-semibold">Device configuration</div>
          <input class="input input-bordered input-sm" placeholder="Device name" bind:value={bfEspDeviceName} />
          <input class="input input-bordered input-sm" placeholder="WiFi SSID" bind:value={bfEspWifiSSID} />
          <input class="input input-bordered input-sm" type="password" placeholder="WiFi password" bind:value={bfEspWifiPassword} />
          <label class="flex items-center gap-2 text-sm cursor-pointer">
            <input type="checkbox" class="checkbox checkbox-sm" bind:checked={bfEspHA} />
            Home Assistant integration (generates API encryption key)
          </label>
          {#if bfEspError}<div class="alert alert-error text-xs">{bfEspError}</div>{/if}
          <div class="flex gap-2 mt-2">
            <button class="btn btn-ghost btn-sm" on:click={() => bfEspStep = 2}>Back</button>
            <button class="btn btn-primary btn-sm"
              disabled={!bfEspDeviceName || !bfEspWifiSSID}
              on:click={() => { bfEspStep = 4; bfEspDoCompile(); }}>Compile</button>
          </div>
        </div>

      {:else if bfEspStep === 4}
        <div class="flex flex-col items-center gap-4 py-8">
          {#if bfEspCompiling}
            <span class="loading loading-spinner loading-lg"></span>
            <span class="text-sm">Submitting compile job…</span>
          {:else if bfEspError}
            <div class="alert alert-error w-full">{bfEspError}</div>
            <button class="btn btn-primary" on:click={bfEspDoCompile}>Retry</button>
          {/if}
        </div>

      {:else if bfEspStep === 5}
        <div class="flex flex-col gap-3">
          <div class="alert alert-success text-sm">Firmware compiled — plug your ESP32 into <strong>this computer</strong> via USB, then click Connect &amp; Flash.</div>
          <p class="text-xs text-base-content/50">The device is already registered in the hub database. This step writes the firmware to flash memory.</p>
          {#if bfEspFlashState === 'error'}
            <div class="alert alert-error text-sm">{bfEspFlashMsg || 'Flash failed'}</div>
          {/if}
          {#if bfEspFlashState !== 'idle' && bfEspFlashState !== 'error' && bfEspFlashState !== 'done'}
            <div class="flex items-center gap-2 text-sm text-base-content/70">
              <span class="loading loading-spinner loading-xs"></span>
              {bfEspFlashMsg}
            </div>
          {/if}
          <div class="flex gap-2 justify-end items-center">
            {#if bfEspFlashState !== 'idle' && bfEspFlashState !== 'error'}
              <button class="btn btn-success btn-sm" on:click={() => { bfEspFlashState = 'done'; bfEspStep = 6; }}>
                ✓ Flash complete — Continue
              </button>
            {/if}
            <esp-web-install-button
              bind:this={bfEspInstallEl}
              manifest="/api/webflash/esphome-manifest?token={bfEspToken}"
            >
              <button slot="activate" class="btn btn-warning btn-sm"
                disabled={bfEspFlashState !== 'idle'}>
                {#if bfEspFlashState !== 'idle' && bfEspFlashState !== 'error'}
                  <span class="loading loading-spinner loading-xs"></span> Flashing…
                {:else}
                  ⚡ Connect &amp; Flash
                {/if}
              </button>
              <span slot="unsupported" class="alert alert-error text-sm">
                Web Serial not supported — use Chrome or Edge.
              </span>
            </esp-web-install-button>
          </div>
        </div>

      {:else if bfEspStep === 6}
        <div class="flex flex-col gap-3">
          <div class="flex items-center gap-3 p-3 rounded-lg border border-success/40 bg-success/10">
            <span class="text-xl">✓</span>
            <div class="flex-1">
              <div class="font-semibold text-sm">{bfEspDeviceName}</div>
              <div class="text-xs text-base-content/50">ESPHome flash complete — device rebooting.</div>
            </div>
          </div>
          {#if bfEspHA}
            <div class="text-xs">The ESPHome API encryption key has been stored. Use the <strong>ESPHome Key</strong> button in the Fleet view to retrieve it for Home Assistant pairing.</div>
          {/if}
          <button class="btn btn-ghost btn-sm self-start mt-2" on:click={bfEspReset}>Flash another device</button>
        </div>
      {/if}
    {/if}

    {/if}
  {/if}

  <!-- ── Serial Debug ───────────────────────────────────────────────────────── -->
  {#if activeTab === 'debug'}
    <div class="flex flex-col gap-4">
      {#if !serialSupported}
        <div class="alert alert-error text-sm">Web Serial API not supported — use Chrome or Edge.</div>
      {:else}
        <div class="flex flex-wrap items-center gap-2">
          <select class="select select-bordered select-xs" bind:value={selectedBaud}
            disabled={dbgStatus !== 'disconnected'}>
            {#each BAUD_RATES as baud}<option value={baud}>{baud}</option>{/each}
          </select>

          {#if dbgStatus === 'disconnected' || dbgStatus === 'error'}
            <button class="btn btn-primary btn-sm" on:click={connect}>Connect</button>
          {:else if dbgStatus === 'connecting'}
            <button class="btn btn-primary btn-sm" disabled>
              <span class="loading loading-spinner loading-xs"></span> Connecting…
            </button>
          {:else}
            <button class="btn btn-error btn-sm" on:click={disconnect}>Disconnect</button>
          {/if}

          <button class="btn btn-ghost btn-sm" on:click={clearLog} disabled={logLines.length === 0}>Clear</button>
          <label class="flex items-center gap-1 text-xs cursor-pointer select-none">
            <input type="checkbox" class="checkbox checkbox-xs" bind:checked={autoScroll} />
            Auto-scroll
          </label>
          <span class="badge {STATUS_BADGE[dbgStatus]} badge-sm ml-auto">{STATUS_LABEL[dbgStatus]}</span>
        </div>

        {#if dbgStatus === 'error' && dbgError}
          <div class="alert alert-error text-xs">{dbgError}</div>
        {/if}

        <pre bind:this={logContainer}
          class="bg-neutral text-neutral-content font-mono text-xs rounded-lg border border-base-300
                 overflow-y-auto h-80 p-3 leading-relaxed whitespace-pre-wrap break-all"
        >{#if logLines.length === 0}<span class="text-base-content/30 italic">No output — connect to see logs.</span
        >{:else}{#each logLines as line}{line + '\n'}{/each}{/if}</pre>

        {#if dbgStatus === 'connected'}
          <form class="flex gap-2" on:submit|preventDefault={sendLine}>
            <input class="input input-bordered input-sm flex-1 font-mono text-xs"
              placeholder="Send command (Enter)…"
              bind:value={inputLine}
              autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" />
            <button type="submit" class="btn btn-sm btn-ghost">Send</button>
          </form>
        {/if}

        <p class="text-xs text-base-content/40">
          Chrome / Edge only. 115200 for ESP32 logs; 74880 for early boot ROM output.
        </p>
      {/if}
    </div>
  {/if}

  <!-- ── Server Flash (existing wizard) ────────────────────────────────── -->
  {#if activeTab === 'server'}
    {#if loadingInit}
      <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
    {:else if error}
      <div class="alert alert-error text-sm">{error}</div>
    {:else}

    <ul class="steps steps-horizontal w-full text-xs">
      <li class="step {step >= 1 ? 'step-primary' : ''}">Template</li>
      <li class="step {step >= 2 ? 'step-primary' : ''}">Names</li>
      <li class="step {step >= 3 ? 'step-primary' : ''}">WiFi &amp; Port</li>
      <li class="step {step >= 4 ? 'step-primary' : ''}">Flash</li>
      <li class="step {step >= 5 ? 'step-primary' : ''}">Done</li>
    </ul>

    {#if step === 1}
      <!-- Firmware type selector — shown at top of step 1 -->
      <div class="flex gap-2 mb-4">
        <button class="btn btn-sm {firmwareType === 'matter' ? 'btn-primary' : 'btn-ghost'}"
          on:click={() => { firmwareType = 'matter'; espStep = 1; }}>Matter</button>
        <button class="btn btn-sm {firmwareType === 'esphome' ? 'btn-primary' : 'btn-ghost'}"
          on:click={() => { firmwareType = 'esphome'; loadESPHomeModules(); }}>ESPHome</button>
      </div>
    {/if}

    {#if firmwareType === 'matter'}
    {#if step === 1}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">Select a template</div>
        {#if templates.length === 0}
          <div class="text-sm text-base-content/50">No templates yet — create one in the Templates view.</div>
        {:else}
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {#each templates as t}
              <button
                class="card p-3 border text-left transition-all
                  {selectedTemplate?.id === t.id ? 'border-primary bg-primary/10' : 'border-base-300 bg-base-200 hover:border-primary/40'}"
                on:click={() => selectedTemplate = t}
              >
                <div class="font-semibold text-sm">{t.name || t.id}</div>
                <div class="text-xs text-base-content/50">{t.board}</div>
              </button>
            {/each}
          </div>
          <button class="btn btn-primary btn-sm self-end" disabled={!selectedTemplate}
            on:click={() => step = 2}>Next →</button>
        {/if}
      </div>

    {:else if step === 2}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">Device names <span class="text-base-content/40 font-normal">(e.g. 1/Bedroom)</span></div>
        {#each deviceNames as _, i}
          <div class="flex gap-2">
            <input class="input input-bordered input-sm flex-1" placeholder="e.g. {i+1}/Room"
              bind:value={deviceNames[i]} />
            {#if deviceNames.length > 1}
              <button class="btn btn-ghost btn-sm" on:click={() => removeName(i)}>✕</button>
            {/if}
          </div>
        {/each}
        <button class="btn btn-ghost btn-sm self-start" on:click={addName}>+ Add device</button>
        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm" on:click={() => step = 1}>← Back</button>
          <button class="btn btn-primary btn-sm"
            disabled={deviceNames.every(n => !n.trim())}
            on:click={() => step = 3}>Next →</button>
        </div>
      </div>

    {:else if step === 3}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">WiFi credentials <span class="text-base-content/40 font-normal">(optional)</span></div>
        <input class="input input-bordered input-sm" placeholder="WiFi SSID" bind:value={wifiSSID} />
        <input class="input input-bordered input-sm" type="password" placeholder="WiFi password" bind:value={wifiPassword} />

        <div class="divider my-1"></div>
        <div class="flex items-center gap-2 text-sm font-semibold">
          USB port
          <button class="btn btn-ghost btn-xs" on:click={refreshPorts}>↻ Refresh</button>
        </div>
        {#if ports.length === 0}
          <div class="text-sm text-base-content/50">No USB ports detected. Plug in your ESP32 and refresh.</div>
        {:else}
          <select class="select select-bordered select-sm" bind:value={selectedPort}>
            <option value="">Select port…</option>
            {#each ports as p}<option value={p.path}>{p.name} ({p.path})</option>{/each}
          </select>
        {/if}

        <div class="divider my-1"></div>
        <div class="text-sm font-semibold">Firmware version</div>
        {#if firmware.length === 0}
          <div class="text-sm text-base-content/50">No firmware uploaded — go to the Firmware view first.</div>
        {:else}
          <select class="select select-bordered select-sm" bind:value={selectedFW}>
            <option value="">Select version…</option>
            {#each firmware as f}<option value={f.version}>{f.version}{f.is_latest ? ' (latest)' : ''}</option>{/each}
          </select>
        {/if}

        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm" on:click={() => step = 2}>← Back</button>
          <button class="btn btn-primary btn-sm"
            disabled={!selectedPort || !selectedFW}
            on:click={() => step = 4}>Next →</button>
        </div>
      </div>

    {:else if step === 4}
      <div class="flex flex-col gap-3">
        <div class="card bg-base-200 border border-base-300 p-4 text-sm space-y-1">
          <div><strong>Template:</strong> {selectedTemplate.name || selectedTemplate.id}</div>
          <div><strong>Devices ({deviceNames.filter(n=>n.trim()).length}):</strong> {deviceNames.filter(n=>n.trim()).join(', ')}</div>
          <div><strong>Port:</strong> {selectedPort}</div>
          <div><strong>Firmware:</strong> {selectedFW}</div>
          <div><strong>WiFi:</strong> {wifiSSID || '— (none)'}</div>
        </div>
        {#if flashError}<div class="alert alert-error text-sm">{flashError}</div>{/if}
        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm" disabled={flashing} on:click={() => step = 3}>← Back</button>
          <button class="btn btn-warning btn-sm" disabled={flashing} on:click={doFlash}>
            {#if flashing}<span class="loading loading-spinner loading-xs"></span> Flashing…{:else}⚡ Flash Now{/if}
          </button>
        </div>
      </div>

    {:else if step === 5}
      <div class="flex flex-col gap-3">
        {#each results as r}
          <div class="flex items-center gap-3 p-3 rounded-lg border {r.ok ? 'border-success/40 bg-success/10' : 'border-error/40 bg-error/10'}">
            <span class="text-xl">{r.ok ? '✓' : '✗'}</span>
            <div class="flex-1">
              <div class="font-semibold text-sm">{r.name}</div>
              {#if r.device_id}<div class="text-xs font-mono text-base-content/50">{r.device_id}</div>{/if}
              {#if r.error}<div class="text-xs text-error mt-0.5">{r.error}</div>{/if}
            </div>
          </div>
        {/each}
        <button class="btn btn-ghost btn-sm self-start mt-2" on:click={reset}>Flash more devices</button>
      </div>
    {/if}
    {/if}

    <!-- ESPHome wizard steps -->
    {#if firmwareType === 'esphome'}

      {#if espStep === 1}
        <!-- Step 1: Board selection -->
        <div class="flex flex-col gap-3">
          <div class="text-sm font-semibold">Select board</div>
          {#each ['esp32-c3', 'esp32-h2', 'esp32', 'esp32-s3'] as board}
            <label class="flex items-center gap-2 cursor-pointer">
              <input type="radio" class="radio radio-sm" bind:group={espBoard} value={board} />
              <span class="text-sm font-mono">{board}</span>
            </label>
          {/each}
          <div class="flex gap-2 mt-2">
            <button class="btn btn-primary btn-sm" on:click={() => espStep = 2}>Next</button>
          </div>
        </div>

      {:else if espStep === 2}
        <!-- Step 2: Component builder -->
        <div class="flex flex-col gap-3">
          <div class="text-sm font-semibold">Add components</div>
          {#each espComponents as comp, i}
            <div class="border border-base-300 rounded p-3 flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-semibold">{comp.type}</span>
                <button class="btn btn-ghost btn-xs" on:click={() => espRemoveComponent(i)}>✕</button>
              </div>
              <input class="input input-bordered input-sm w-full" placeholder="Name (e.g. Room Temp)"
                bind:value={comp.name} />
              {#each Object.keys(comp.pins) as role}
                {@const pinDef = (comp.io || []).find(p => p.id === role)}
                <label class="text-xs flex items-center gap-2">
                  <span class="w-24 font-mono">{role}</span>
                  {#if pinDef?.type === 'config'}
                    <input class="input input-bordered input-xs flex-1" type="text"
                      placeholder="integer" bind:value={comp.pins[role]} />
                  {:else}
                    <select class="select select-bordered select-xs flex-1" bind:value={comp.pins[role]}>
                      <option value="">Select GPIO…</option>
                      {#each boardGpios(espBoard) as gpio}
                        <option value={gpio}>{gpio}</option>
                      {/each}
                    </select>
                  {/if}
                </label>
              {/each}
            </div>
          {/each}
          <select class="select select-bordered select-sm" on:change={e => { espAddComponent(e.target.value); e.target.value = ''; }}>
            <option value="">+ Add component…</option>
            {#each espModules as m}
              <option value={m.id}>{m.name}</option>
            {/each}
          </select>
          <div class="flex gap-2 mt-2">
            <button class="btn btn-ghost btn-sm" on:click={() => espStep = 1}>Back</button>
            <button class="btn btn-primary btn-sm" disabled={espComponents.length === 0}
              on:click={() => espStep = 3}>Next</button>
          </div>
        </div>

      {:else if espStep === 3}
        <!-- Step 3: Device config -->
        <div class="flex flex-col gap-3">
          <div class="text-sm font-semibold">Device configuration</div>
          <input class="input input-bordered input-sm" placeholder="Device name" bind:value={espDeviceName} />
          <input class="input input-bordered input-sm" placeholder="WiFi SSID" bind:value={espWifiSSID} />
          <input class="input input-bordered input-sm" type="password" placeholder="WiFi password" bind:value={espWifiPassword} />
          <label class="flex items-center gap-2 text-sm cursor-pointer">
            <input type="checkbox" class="checkbox checkbox-sm" bind:checked={espHA} />
            Home Assistant integration (generates API encryption key)
          </label>
          {#if espFlashError}<div class="alert alert-error text-xs">{espFlashError}</div>{/if}
          <div class="flex gap-2 mt-2">
            <button class="btn btn-ghost btn-sm" on:click={() => espStep = 2}>Back</button>
            <button class="btn btn-primary btn-sm"
              disabled={!espDeviceName || !espWifiSSID || !selectedPort}
              on:click={() => { espStep = 4; espDoFlash(); }}>Flash</button>
          </div>
          {#if !selectedPort}
            <div class="text-xs text-warning">Select a USB port in WiFi &amp; Port step first.</div>
          {/if}
        </div>

      {:else if espStep === 4}
        <!-- Step 4: Compile + flash progress -->
        <div class="flex flex-col gap-3">
          <div class="text-sm font-semibold">
            {espFlashing ? 'Compiling + flashing… (may take several minutes on first flash)' : espFlashError ? 'Compilation failed' : 'Done'}
          </div>
          <div class="bg-base-300 rounded p-2 h-48 overflow-y-auto font-mono text-xs">
            {#each espLogs as line}<div>{line}</div>{/each}
            {#if espFlashing}<div class="animate-pulse">▋</div>{/if}
          </div>
          {#if espFlashError}
            <div class="alert alert-error text-xs">{espFlashError}</div>
            <div class="flex gap-2">
              <button class="btn btn-ghost btn-sm" on:click={() => espStep = 3}>← Back</button>
              <button class="btn btn-warning btn-sm" on:click={espDoFlash}>↺ Retry</button>
            </div>
          {/if}
        </div>

      {:else if espStep === 5 || espStep === 6}
        <!-- Step 6: Done -->
        <div class="flex flex-col gap-3">
          {#if espResult?.ok}
            <div class="alert alert-success text-sm">Device flashed successfully — {espResult.name}</div>
            {#if espHA}
              <div class="text-xs">The ESPHome API encryption key has been stored. Use the <strong>ESPHome Key</strong> button in the Fleet view to retrieve it for Home Assistant pairing.</div>
            {/if}
          {:else if espResult}
            <div class="alert alert-error text-sm">{espResult.error}</div>
          {/if}
          <button class="btn btn-ghost btn-sm self-start" on:click={espReset}>Flash another device</button>
        </div>
      {/if}

    {/if}

    {/if}
  {/if}
</div>
