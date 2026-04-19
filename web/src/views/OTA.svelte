<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';

  let devices = [];
  let latestFW = null;
  let error = '';
  let loading = true;

  let historyDevice = null;
  let historyLog = [];
  let historyLoading = false;
  let historyError = '';

  let interval;

  onMount(async () => {
    await load();
    interval = setInterval(load, 15000);
  });

  onDestroy(() => clearInterval(interval));

  async function load() {
    try {
      const [devs, fwList] = await Promise.all([
        api.get('/api/devices'),
        api.get('/api/firmware'),
      ]);
      devices = devs || [];
      latestFW = fwList?.find(f => f.is_latest) ?? null;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function normalizeVer(v) { return (v || '').replace(/^v/i, '').trim(); }

  function needsUpdate(dev) {
    return latestFW && dev.fw_version &&
      normalizeVer(dev.fw_version) !== normalizeVer(latestFW.version);
  }

  function statusBadge(dev) {
    if (dev.status === 'online') return 'badge-success';
    if (dev.status === 'offline') return 'badge-error';
    return 'badge-ghost';
  }

  function fwBadge(dev) {
    if (!latestFW) return 'badge-ghost';
    if (!dev.fw_version) return 'badge-ghost';
    if (normalizeVer(dev.fw_version) === normalizeVer(latestFW.version)) return 'badge-success';
    return 'badge-warning';
  }

  async function openHistory(dev) {
    historyDevice = dev;
    historyLog = [];
    historyError = '';
    historyLoading = true;
    try {
      historyLog = await api.get(`/api/ota/log/${dev.id}`);
    } catch (e) {
      historyError = e.message;
    } finally {
      historyLoading = false;
    }
  }

  function closeHistory() { historyDevice = null; historyLog = []; historyError = ''; }

  $: outdatedCount = devices.filter(d => needsUpdate(d)).length;
  $: upToDateCount = latestFW ? devices.filter(d => normalizeVer(d.fw_version) === normalizeVer(latestFW.version)).length : 0;
</script>

{#if historyDevice}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" on:click={closeHistory} />
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-lg flex flex-col max-h-[80vh]">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">OTA History — {historyDevice.name}</span>
        <button class="btn btn-ghost btn-xs" on:click={closeHistory}>✕</button>
      </div>
      <div class="flex-1 overflow-auto p-4">
        {#if historyLoading}
          <div class="flex justify-center py-8"><span class="loading loading-spinner"></span></div>
        {:else if historyError}
          <div class="alert alert-error text-sm">{historyError}</div>
        {:else if historyLog.length === 0}
          <div class="text-sm text-base-content/50 text-center py-6">No OTA updates recorded yet.</div>
        {:else}
          <table class="table table-sm">
            <thead>
              <tr><th>Date</th><th>From</th><th>To</th><th>Result</th></tr>
            </thead>
            <tbody>
              {#each historyLog as e (e.id)}
                <tr>
                  <td class="text-xs text-base-content/50">{new Date(e.created_at).toLocaleString()}</td>
                  <td class="font-mono text-xs">{e.from_ver || '—'}</td>
                  <td class="font-mono text-xs">{e.to_ver}</td>
                  <td>
                    <span class="badge badge-xs {e.result === 'ok' ? 'badge-success' : 'badge-error'}">{e.result}</span>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>
      <div class="px-5 pb-4 flex justify-end">
        <button class="btn btn-ghost btn-sm" on:click={closeHistory}>Close</button>
      </div>
    </div>
  </div>
{/if}

<div class="p-6 flex flex-col gap-6 max-w-4xl">
  <div class="flex items-center justify-between flex-wrap gap-2">
    <h2 class="text-lg font-semibold">OTA Updates</h2>
    {#if latestFW}
      <span class="text-sm text-base-content/60">
        Latest firmware: <span class="font-mono font-semibold">{latestFW.version}</span>
      </span>
    {/if}
  </div>

  {#if !latestFW && !loading}
    <div class="alert alert-warning text-sm">
      No firmware marked as latest. Upload and mark a version in the <strong>Firmware</strong> view.
    </div>
  {/if}

  {#if latestFW && outdatedCount > 0}
    <div class="alert alert-info text-sm">
      {outdatedCount} device{outdatedCount !== 1 ? 's' : ''} running outdated firmware — they will update automatically on next check-in.
    </div>
  {/if}

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if devices.length === 0}
    <div class="text-sm text-base-content/50 text-center py-8">No devices registered yet.</div>
  {:else}
    <div class="overflow-x-auto rounded-lg border border-base-200">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Device</th>
            <th>Status</th>
            <th>Current FW</th>
            <th>Latest FW</th>
            <th>Last Seen</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each devices as d (d.id)}
            <tr class="hover">
              <td>
                <div class="font-mono text-sm font-semibold">{d.name}</div>
                <div class="text-xs text-base-content/40">{d.id}</div>
              </td>
              <td><span class="badge badge-sm {statusBadge(d)}">{d.status}</span></td>
              <td><span class="badge badge-sm font-mono {fwBadge(d)}">{d.fw_version || '—'}</span></td>
              <td class="font-mono text-sm text-base-content/60">{latestFW ? latestFW.version : '—'}</td>
              <td class="text-sm text-base-content/50">
                {d.last_seen ? new Date(d.last_seen).toLocaleString() : '—'}
              </td>
              <td>
                <button class="btn btn-xs btn-ghost" on:click={() => openHistory(d)}>History</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="flex gap-4 text-sm text-base-content/60">
      <span>✓ Up to date: <strong>{upToDateCount}</strong></span>
      <span>⚠ Outdated: <strong>{outdatedCount}</strong></span>
      <span>Total: <strong>{devices.length}</strong></span>
    </div>
    <p class="text-xs text-base-content/40">Devices poll the OTA server automatically. Outdated devices download the latest firmware on next check-in.</p>
  {/if}
</div>
