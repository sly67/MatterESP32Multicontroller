<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';

  let health = null;
  let error = '';

  onMount(async () => {
    try {
      health = await api.get('/api/health');
    } catch (e) {
      error = e.message;
    }
  });
</script>

<div class="p-6 flex flex-col gap-6 max-w-lg">
  <h2 class="text-lg font-semibold">Settings</h2>

  <div class="card bg-base-200 border border-base-300 p-4">
    <div class="text-sm font-semibold mb-3">System Health</div>
    {#if error}
      <div class="alert alert-error text-xs">{error}</div>
    {:else if health}
      <div class="flex items-center gap-2">
        <span class="badge badge-success badge-sm">●</span>
        <span class="text-sm">API reachable — status: <strong>{health.status}</strong></span>
      </div>
    {:else}
      <span class="loading loading-dots loading-sm"></span>
    {/if}
  </div>

  <div class="card bg-base-200 border border-base-300 p-4">
    <div class="text-sm font-semibold mb-2">About</div>
    <div class="text-xs text-base-content/60 space-y-1">
      <div>Web UI port: <span class="font-mono">48060</span></div>
      <div>OTA port: <span class="font-mono">48061</span></div>
      <div>Database: SQLite (WAL mode)</div>
      <div>Transport: HTTPS (self-signed TLS)</div>
    </div>
  </div>

  <div class="card bg-base-200 border border-base-300 p-4">
    <div class="text-sm font-semibold mb-2">Flash / OTA Configuration</div>
    <div class="text-xs text-base-content/50">Available in Plan 4 (USB flashing) and Plan 5 (OTA server).</div>
  </div>
</div>
