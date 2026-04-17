<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';

  let devices = [];
  let error = '';
  let filter = '';
  let loading = true;

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
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
