<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';

  let devices = [];
  let latestFW = null;
  let error = '';
  let loading = true;

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

  function needsUpdate(dev) {
    return latestFW && dev.fw_version && dev.fw_version !== latestFW.version;
  }

  function statusBadge(dev) {
    if (dev.status === 'online') return 'badge-success';
    if (dev.status === 'offline') return 'badge-error';
    return 'badge-ghost';
  }

  function fwBadge(dev) {
    if (!latestFW) return 'badge-ghost';
    if (!dev.fw_version) return 'badge-ghost';
    if (dev.fw_version === latestFW.version) return 'badge-success';
    return 'badge-warning';
  }

  $: outdatedCount = devices.filter(d => needsUpdate(d)).length;
  $: upToDateCount = latestFW ? devices.filter(d => d.fw_version === latestFW.version).length : 0;
</script>

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
