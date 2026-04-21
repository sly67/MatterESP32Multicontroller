<script>
  import { onMount } from 'svelte';
  import QRCode from 'qrcode';
  import { api } from '../lib/api.js';

  let devices = [];
  let error = '';
  let filter = '';
  let loading = true;
  let latestJobs = {};

  let pairModal = null; // { discriminator, passcode, qr_payload } | null
  let qrDataUrl = '';
  let pairError = '';
  let deleteConfirm = null; // device to delete
  let deleteError = '';

  async function removeDevice(d) {
    try {
      await api.delete(`/api/devices/${d.id}`);
      devices = devices.filter(x => x.id !== d.id);
      deleteConfirm = null;
    } catch (e) {
      deleteError = e.message;
    }
  }

  onMount(async () => {
    try {
      devices = await api.get('/api/devices');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
    try {
      const jobs = await fetch('/api/jobs').then(r => r.json());
      const seen = new Set();
      for (const j of jobs) {
        if (j.device_id && !seen.has(j.device_id)) {
          seen.add(j.device_id);
          latestJobs[j.device_id] = j;
        }
      }
      latestJobs = { ...latestJobs };
    } catch (_) {}
  });

  const jobBadgeClass = s => ({
    pending:   'badge-warning',
    running:   'badge-info',
    done:      'badge-success',
    failed:    'badge-error',
    cancelled: 'badge-ghost',
  }[s] || 'badge-ghost');

  $: filtered = devices.filter(d =>
    (d.name || '').toLowerCase().includes(filter.toLowerCase()) ||
    (d.status || '').toLowerCase().includes(filter.toLowerCase())
  );

  const statusClass = s => ({
    online:  'badge-success',
    offline: 'badge-error',
  }[s] || 'badge-ghost');

  async function openPairModal(device) {
    pairError = '';
    try {
      const res = await api.get(`/api/devices/${device.id}/pairing`);
      pairModal = res;
      qrDataUrl = await QRCode.toDataURL(res.qr_payload, { width: 220, margin: 2 });
    } catch (e) {
      pairError = e.message;
    }
  }

  function closePairModal() { pairModal = null; qrDataUrl = ''; pairError = ''; }

  let espKeyModal = null; // { api_key, ota_password } | null
  let espKeyError = '';

  async function openESPHomeKey(device) {
    espKeyError = '';
    try {
      espKeyModal = await api.get(`/api/devices/${device.id}/esphome-key`);
    } catch (e) {
      espKeyError = e.message;
    }
  }
  function closeESPHomeKey() { espKeyModal = null; espKeyError = ''; }

  function copyToClipboard(text) {
    navigator.clipboard.writeText(text).catch(() => {});
  }
</script>

<div class="p-6 flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold">Fleet</h2>
    <span class="text-sm text-base-content/50">{devices.length} device{devices.length !== 1 ? 's' : ''}</span>
  </div>

  <input
    class="input input-bordered input-sm w-full max-w-xs"
    placeholder="Filter by name or status…"
    bind:value={filter}
  />

  {#if pairError}
    <div class="alert alert-error text-sm">{pairError}</div>
  {/if}

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if filtered.length === 0}
    <div class="text-sm text-base-content/50 py-8 text-center">
      {devices.length === 0 ? 'No devices registered yet. Flash a device to get started.' : 'No devices match the filter.'}
    </div>
  {:else}
    <div class="overflow-x-auto rounded-lg border border-base-200">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Type / Template</th>
            <th>Firmware</th>
            <th>IP</th>
            <th>Last Seen</th>
            <th>Flashed</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as d (d.id)}
            <tr class="hover">
              <td class="font-mono text-sm">{d.name}</td>
              <td><span class="badge badge-sm {statusClass(d.status)}">{d.status}</span></td>
              <td class="text-sm text-base-content/70">
                {#if d.firmware_type === 'esphome'}
                  <span class="badge badge-xs badge-info mr-1">ESPHome</span>{d.esphome_board || ''}
                {:else}
                  {d.template_id || '—'}
                {/if}
              </td>
              <td class="text-sm font-mono">
                {#if d.firmware_type === 'esphome'}
                  <span class="text-base-content/40">ESPHome</span>
                {:else}
                  {d.fw_version || '—'}
                {/if}
              </td>
              <td class="text-sm font-mono">{d.ip || '—'}</td>
              <td class="text-sm text-base-content/50">{d.last_seen ? new Date(d.last_seen).toLocaleString() : '—'}</td>
              <td class="text-sm text-base-content/50">{d.created_at ? new Date(d.created_at).toLocaleDateString() : '—'}</td>
              <td class="flex gap-1 flex-wrap items-center">
                {#if d.firmware_type === 'esphome'}
                  <button class="btn btn-xs btn-outline" on:click={() => openESPHomeKey(d)}>Key</button>
                {:else}
                  <button class="btn btn-xs btn-outline" on:click={() => openPairModal(d)}>Pair</button>
                {/if}
                {#if latestJobs[d.id]}
                  {@const lj = latestJobs[d.id]}
                  <button class="badge badge-sm {jobBadgeClass(lj.status)} cursor-pointer"
                    on:click={() => window.dispatchEvent(new CustomEvent('navigate', { detail: { view: 'jobmonitor', jobId: lj.id } }))}>
                    ESPHome: {lj.status}
                  </button>
                {/if}
                <button class="btn btn-xs btn-error btn-outline" on:click={() => { deleteConfirm = d; deleteError = ''; }}>✕</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

{#if pairModal}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" on:click={closePairModal} />
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-sm flex flex-col">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">Commission device</span>
        <button class="btn btn-ghost btn-xs" on:click={closePairModal}>✕</button>
      </div>
      <div class="flex flex-col items-center gap-4 p-5">
        <p class="text-xs text-base-content/60 text-center">
          Scan with Apple Home, Google Home, or any Matter controller.<br>
          Device must be in commissioning mode (first boot after flash).
        </p>
        {#if qrDataUrl}
          <img src={qrDataUrl} alt="Matter QR code" class="rounded-lg border border-base-300" width="220" height="220" />
        {:else}
          <span class="loading loading-spinner loading-md"></span>
        {/if}
        <div class="w-full text-xs font-mono bg-base-300 rounded p-3 space-y-1">
          <div><span class="text-base-content/50">Discriminator:</span> {pairModal.discriminator}</div>
          <div><span class="text-base-content/50">Passcode:</span> {pairModal.passcode}</div>
          <div class="break-all"><span class="text-base-content/50">QR payload:</span> {pairModal.qr_payload}</div>
        </div>
      </div>
      <div class="px-5 pb-4 flex justify-end">
        <button class="btn btn-ghost btn-sm" on:click={closePairModal}>Close</button>
      </div>
    </div>
  </div>
{/if}

{#if espKeyModal}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" on:click={closeESPHomeKey} />
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-sm flex flex-col">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">ESPHome credentials</span>
        <button class="btn btn-ghost btn-xs" on:click={closeESPHomeKey}>✕</button>
      </div>
      <div class="flex flex-col gap-3 p-5">
        <p class="text-xs text-base-content/60">Use the API key in Home Assistant → Add Integration → ESPHome.</p>
        <div class="w-full text-xs font-mono bg-base-300 rounded p-3 space-y-2">
          <div class="flex items-center justify-between gap-2">
            <div>
              <div class="text-base-content/50">API Encryption Key</div>
              <div class="break-all">{espKeyModal.api_key}</div>
            </div>
            <button class="btn btn-ghost btn-xs shrink-0"
              on:click={() => copyToClipboard(espKeyModal.api_key)}>Copy</button>
          </div>
          <div class="flex items-center justify-between gap-2">
            <div>
              <div class="text-base-content/50">OTA Password</div>
              <div class="break-all">{espKeyModal.ota_password}</div>
            </div>
            <button class="btn btn-ghost btn-xs shrink-0"
              on:click={() => copyToClipboard(espKeyModal.ota_password)}>Copy</button>
          </div>
        </div>
      </div>
      <div class="px-5 pb-4 flex justify-end">
        <button class="btn btn-ghost btn-sm" on:click={closeESPHomeKey}>Close</button>
      </div>
    </div>
  </div>
{/if}

{#if deleteConfirm}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" on:click={() => { deleteConfirm = null; deleteError = ''; }} />
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-sm flex flex-col">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">Remove device</span>
        <button class="btn btn-ghost btn-xs" on:click={() => { deleteConfirm = null; deleteError = ''; }}>✕</button>
      </div>
      <div class="flex flex-col gap-3 p-5">
        <p class="text-sm">Remove <span class="font-mono font-semibold">{deleteConfirm.name}</span> from the fleet?</p>
        <p class="text-xs text-base-content/50">This only removes the device record — the physical device is unaffected.</p>
        {#if deleteError}
          <div class="alert alert-error text-xs">{deleteError}</div>
        {/if}
      </div>
      <div class="px-5 pb-4 flex justify-end gap-2">
        <button class="btn btn-ghost btn-sm" on:click={() => { deleteConfirm = null; deleteError = ''; }}>Cancel</button>
        <button class="btn btn-error btn-sm" on:click={() => removeDevice(deleteConfirm)}>Delete</button>
      </div>
    </div>
  </div>
{/if}

{#if espKeyError}
  <div class="toast toast-end"><div class="alert alert-error text-xs">{espKeyError}</div></div>
{/if}
