<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';

  // ── Tab ────────────────────────────────────────────────────────────────────
  let activeTab = 'server'; // 'server' | 'browser'

  // ── Browser Flash ──────────────────────────────────────────────────────────
  import 'esp-web-tools';

  let browserFlashState = 'idle'; // idle | connecting | writing | done | error
  let browserFlashMsg = '';
  let firmwareAvailable = false;
  let latestVersion = '';

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
  }

  function handleInstallEvent(e) {
    const state = e.detail?.state;
    if (!state) return;
    if (state === 'finished') {
      browserFlashState = 'done';
      browserFlashMsg = '';
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
  </div>

  <!-- ── Browser Flash ──────────────────────────────────────────────────── -->
  {#if activeTab === 'browser'}
    <div class="flex flex-col gap-4">
      <div class="text-sm text-base-content/60">
        Plug your ESP32-C3 into <strong>your computer</strong> via USB, then click the button below.
        The browser will flash the latest hub firmware directly over the serial connection.<br>
        <span class="text-warning text-xs">Requires Chrome or Edge (Web Serial API).</span>
      </div>

      {#if !firmwareAvailable}
        <div class="alert alert-warning text-sm">
          No firmware marked as latest. Upload and set a firmware version in the
          <strong>Firmware</strong> view first.
        </div>
      {:else}
        <div class="flex items-center gap-3 p-3 rounded-lg bg-base-200 border border-base-300 text-sm">
          <span class="text-base-content/50">Firmware to flash:</span>
          <span class="font-mono font-semibold">{latestVersion}</span>
        </div>

        {#if browserFlashState === 'done'}
          <div class="alert alert-success text-sm">
            Flash complete! The device will reboot into the Matter hub firmware.
            Use the <strong>Server Flash</strong> tab to provision it with WiFi and device credentials.
          </div>
          <button class="btn btn-ghost btn-sm self-start"
            on:click={() => { browserFlashState = 'idle'; browserFlashMsg = ''; }}>
            Flash another device
          </button>

        {:else if browserFlashState === 'error'}
          <div class="alert alert-error text-sm">{browserFlashMsg || 'Flash failed'}</div>
          <button class="btn btn-ghost btn-sm self-start"
            on:click={() => { browserFlashState = 'idle'; browserFlashMsg = ''; }}>
            Try again
          </button>

        {:else}
          {#if browserFlashState !== 'idle'}
            <div class="flex items-center gap-2 text-sm text-base-content/70">
              <span class="loading loading-spinner loading-xs"></span>
              {browserFlashMsg}
            </div>
          {/if}

          <esp-web-install-button
            manifest="/api/webflash/manifest.json"
            on:state-changed={handleInstallEvent}
          >
            <button
              slot="activate"
              class="btn btn-primary"
              disabled={browserFlashState !== 'idle'}
            >
              Connect &amp; Flash ESP32-C3
            </button>
            <span slot="unsupported" class="alert alert-error text-sm">
              Web Serial is not supported in this browser. Use Chrome or Edge.
            </span>
          </esp-web-install-button>
        {/if}
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
        <div class="text-sm font-semibold">WiFi credentials</div>
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
            disabled={!wifiSSID || !selectedPort || !selectedFW}
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
          <div><strong>WiFi:</strong> {wifiSSID}</div>
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
  {/if}
</div>
