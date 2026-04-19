<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { api } from '../lib/api.js';

  // ── firmware list ──────────────────────────────────────────────────────────
  let versions = [];
  let error = '';
  let loading = true;
  let firmwareError = '';

  // ── manual upload form ─────────────────────────────────────────────────────
  let uploading = false;
  let uploadError = '';
  let version = '';
  let boards = '';
  let notes = '';
  let file = null;

  // ── builder ────────────────────────────────────────────────────────────────
  let buildState = 'idle';   // 'idle' | 'running' | 'done' | 'error'
  let buildLines = [];
  let buildError = '';
  let buildId = null;
  let es = null;             // EventSource
  let logEl;                 // bind:this on log <pre>

  onMount(async () => {
    await load();
    await checkRunningBuild();
  });

  onDestroy(() => { es?.close(); });

  // ── data ───────────────────────────────────────────────────────────────────
  async function load() {
    loading = true;
    try {
      const res = await fetch('/api/firmware');
      if (!res.ok) throw new Error(await res.text());
      versions = await res.json();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function checkRunningBuild() {
    try {
      const res = await fetch('/api/build/status');
      const data = await res.json();
      if (data.running && data.id) {
        buildState = 'running';
        buildId = data.id;
        attachStream(data.id);
      }
    } catch {}
  }

  // ── upload ─────────────────────────────────────────────────────────────────
  async function upload() {
    if (!version || !boards || !file) {
      uploadError = 'Version, boards and file are required.';
      return;
    }
    uploadError = '';
    uploading = true;
    try {
      const fd = new FormData();
      fd.append('version', version);
      fd.append('boards', boards);
      fd.append('notes', notes);
      fd.append('file', file);
      const res = await fetch('/api/firmware', { method: 'POST', body: fd });
      if (!res.ok) throw new Error(await res.text());
      version = ''; boards = ''; notes = ''; file = null;
      await load();
    } catch (e) {
      uploadError = e.message;
    } finally {
      uploading = false;
    }
  }

  async function setLatest(v) {
    firmwareError = '';
    try {
      await api.post(`/api/firmware/${v}/set-latest`, undefined);
      await load();
    } catch (e) {
      firmwareError = e.message;
    }
  }

  async function remove(v) {
    if (!confirm(`Delete firmware ${v}?`)) return;
    firmwareError = '';
    try {
      await api.delete(`/api/firmware/${v}`);
      await load();
    } catch (e) {
      firmwareError = e.message;
    }
  }

  // ── builder ────────────────────────────────────────────────────────────────
  async function startBuild() {
    buildLines = [];
    buildError = '';
    buildState = 'running';

    try {
      const res = await fetch('/api/build', { method: 'POST' });
      if (!res.ok) {
        buildState = 'error';
        buildError = await res.text();
        return;
      }
      const data = await res.json();
      buildId = data.id;
      attachStream(buildId);
    } catch (e) {
      buildState = 'error';
      buildError = e.message;
    }
  }

  function attachStream(id) {
    es?.close();
    es = new EventSource(`/api/build/${id}/logs`);

    es.onmessage = async (e) => {
      buildLines = [...buildLines, e.data];
      await tick();
      if (logEl) logEl.scrollTop = logEl.scrollHeight;
    };

    es.addEventListener('done', async () => {
      buildState = 'done';
      es.close(); es = null;
      // Give watcher time to pick up the binary (polls every 5 s + 1 s stable check).
      setTimeout(load, 7000);
    });

    // Named 'fail' event avoids conflict with EventSource's built-in 'error' event.
    es.addEventListener('fail', async (e) => {
      buildState = 'error';
      buildError = e.data || 'Build failed';
      es.close(); es = null;
    });

    es.onerror = () => {
      if (buildState === 'running') {
        buildState = 'error';
        buildError = 'Connection to server lost during build.';
      }
      es?.close(); es = null;
    };
  }

  const BUILD_BADGE = {
    idle:    '',
    running: 'badge-warning',
    done:    'badge-success',
    error:   'badge-error',
  };
  const BUILD_LABEL = {
    idle:    'Idle',
    running: 'Building…',
    done:    'Done',
    error:   'Error',
  };
</script>

<div class="p-6 flex flex-col gap-6 max-w-3xl">
  <h2 class="text-lg font-semibold">Firmware</h2>

  <!-- ── Manual upload ──────────────────────────────────────────────────── -->
  <div class="card bg-base-200 border border-base-300 p-4 flex flex-col gap-3">
    <div class="text-sm font-semibold">Upload Binary</div>
    {#if uploadError}<div class="alert alert-error text-xs">{uploadError}</div>{/if}
    <div class="grid grid-cols-2 gap-2">
      <input class="input input-bordered input-sm" placeholder="Version (e.g. 1.2.0)" bind:value={version} />
      <input class="input input-bordered input-sm" placeholder="Boards (e.g. esp32c3)" bind:value={boards} />
    </div>
    <input class="input input-bordered input-sm" placeholder="Release notes (optional)" bind:value={notes} />
    <input type="file" accept=".bin" class="file-input file-input-bordered file-input-sm"
      on:change={e => file = e.target.files[0]} />
    <button class="btn btn-primary btn-sm self-start" on:click={upload} disabled={uploading}>
      {uploading ? 'Uploading…' : 'Upload'}
    </button>
  </div>

  <!-- ── Auto-drop watcher hint ─────────────────────────────────────────── -->
  <div class="alert alert-info text-xs">
    <span>
      <strong>Auto-registration:</strong> drop any <code>.bin</code> into
      <code>/Portainer/MatterESP32/firmware/incoming/</code> — the server picks it up
      within 5 s, registers it, and sets it as latest. Version is parsed from the
      filename (<code>v1.2.3</code> pattern) or auto-generated from timestamp.
    </span>
  </div>

  <!-- ── Builder ────────────────────────────────────────────────────────── -->
  <div class="card bg-base-200 border border-base-300 p-4 flex flex-col gap-3">
    <div class="flex items-center gap-3">
      <div class="text-sm font-semibold flex-1">Build from Source</div>
      {#if buildState !== 'idle'}
        <span class="badge {BUILD_BADGE[buildState]} badge-sm">{BUILD_LABEL[buildState]}</span>
      {/if}
    </div>

    <p class="text-xs text-base-content/60">
      Requires <code>FIRMWARE_SRC_DIR</code> env var + <code>/var/run/docker.sock</code>
      mounted. See docker-compose.yml. Build takes 30-60 min on first run (Matter SDK);
      subsequent builds use the cached Docker layer and finish in ~3 min.
    </p>

    <div class="flex gap-2">
      <button class="btn btn-secondary btn-sm"
        on:click={startBuild}
        disabled={buildState === 'running'}>
        {#if buildState === 'running'}
          <span class="loading loading-spinner loading-xs"></span> Building…
        {:else}
          Build Firmware
        {/if}
      </button>
      {#if buildLines.length > 0}
        <button class="btn btn-ghost btn-sm" on:click={() => buildLines = []}>Clear log</button>
      {/if}
      <button class="btn btn-ghost btn-sm ml-auto" on:click={load}>↻ Refresh list</button>
    </div>

    {#if buildState === 'error' && buildError}
      <div class="alert alert-error text-xs">{buildError}</div>
    {/if}

    {#if buildLines.length > 0}
      <pre bind:this={logEl}
        class="bg-neutral text-neutral-content font-mono text-xs rounded-lg border
               border-base-300 overflow-y-auto h-72 p-3 leading-relaxed whitespace-pre-wrap break-all"
      >{buildLines.join('\n')}</pre>
    {/if}

    {#if buildState === 'done'}
      <div class="alert alert-success text-xs">
        Build complete — firmware auto-registered within 5 s. List refreshes automatically.
      </div>
    {/if}
  </div>

  <!-- ── Firmware list ──────────────────────────────────────────────────── -->
  {#if firmwareError}
    <div class="alert alert-error text-sm">{firmwareError}</div>
  {/if}

  {#if loading}
    <div class="flex justify-center py-8"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if versions.length === 0}
    <div class="text-sm text-base-content/50 text-center py-6">No firmware uploaded yet.</div>
  {:else}
    <div class="overflow-x-auto rounded-lg border border-base-200">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Version</th><th>Boards</th><th>Notes</th><th>Status</th><th></th>
          </tr>
        </thead>
        <tbody>
          {#each versions as fw (fw.version)}
            <tr class="hover">
              <td class="font-mono text-sm">{fw.version}</td>
              <td class="text-xs">{fw.boards}</td>
              <td class="text-xs text-base-content/60">
                {#if fw.notes === 'auto-registered from incoming/'}
                  <span class="badge badge-info badge-xs mr-1">auto</span>
                {/if}
                {fw.notes || '—'}
              </td>
              <td>
                {#if fw.is_latest}
                  <span class="badge badge-success badge-sm">latest</span>
                {/if}
              </td>
              <td class="flex gap-1 justify-end">
                {#if !fw.is_latest}
                  <button class="btn btn-ghost btn-xs" on:click={() => setLatest(fw.version)}>
                    Set latest
                  </button>
                {/if}
                <button class="btn btn-error btn-xs" on:click={() => remove(fw.version)}>✕</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
