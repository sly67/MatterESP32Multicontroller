<script>
  import { onMount } from 'svelte';
  import QRCode from 'qrcode';
  import { api } from '../lib/api.js';

  let devices = [];
  let error = '';
  let filter = '';
  let loading = true;

  let pairModal = null; // { discriminator, passcode, qr_payload } | null
  let qrDataUrl = '';
  let pairError = '';

  onMount(async () => {
    try {
      devices = await api.get('/api/devices');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

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
            <th>Template</th>
            <th>Firmware</th>
            <th>IP</th>
            <th>Last Seen</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as d (d.id)}
            <tr class="hover">
              <td class="font-mono text-sm">{d.name}</td>
              <td><span class="badge badge-sm {statusClass(d.status)}">{d.status}</span></td>
              <td class="text-sm text-base-content/70">{d.template_id}</td>
              <td class="text-sm font-mono">{d.fw_version || '—'}</td>
              <td class="text-sm font-mono">{d.ip || '—'}</td>
              <td class="text-sm text-base-content/50">{d.last_seen ? new Date(d.last_seen).toLocaleString() : '—'}</td>
              <td>
                {#if d.firmware_type === 'esphome'}
                  <button class="btn btn-xs btn-outline" on:click={() => openESPHomeKey(d)}>ESPHome Key</button>
                {:else}
                  <button class="btn btn-xs btn-outline" on:click={() => openPairModal(d)}>Pair</button>
                {/if}
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

{#if espKeyError}
  <div class="toast toast-end"><div class="alert alert-error text-xs">{espKeyError}</div></div>
{/if}
