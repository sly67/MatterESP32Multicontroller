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
    d.name.toLowerCase().includes(filter.toLowerCase()) ||
    d.status.toLowerCase().includes(filter.toLowerCase())
  );

  const statusClass = s => ({
    online:  'badge-success',
    offline: 'badge-error',
  }[s] || 'badge-ghost');

  async function openPairModal(device) {
    const res = await fetch(`/api/devices/${device.id}/pairing`).then(r => r.json());
    pairModal = res;
    qrDataUrl = await QRCode.toDataURL(res.qr_payload, { width: 220, margin: 2 });
  }

  function closePairModal() { pairModal = null; qrDataUrl = ''; }
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
              <td><button class="btn btn-xs btn-outline" on:click={() => openPairModal(d)}>Pair</button></td>
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
