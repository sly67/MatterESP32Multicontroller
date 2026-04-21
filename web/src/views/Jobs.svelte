<script>
  import { onMount } from 'svelte';

  let jobs = [];
  let error = '';
  let loading = true;

  onMount(async () => {
    try {
      const res = await fetch('/api/jobs');
      if (!res.ok) throw new Error(await res.text());
      jobs = await res.json();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  const statusClass = {
    pending:   'badge-warning',
    running:   'badge-info',
    done:      'badge-success',
    failed:    'badge-error',
    cancelled: 'badge-ghost',
  };

  function openJob(id) {
    window.dispatchEvent(new CustomEvent('navigate', { detail: { view: 'jobmonitor', jobId: id } }));
  }

  async function cancelJob(id, e) {
    e.stopPropagation();
    await fetch(`/api/jobs/${id}`, { method: 'DELETE' });
    jobs = jobs.map(j => j.id === id ? { ...j, status: 'cancelled' } : j);
  }

  async function resubmit(id, e) {
    e.stopPropagation();
    const res = await fetch(`/api/jobs/${id}/resubmit`, { method: 'POST' });
    const d = await res.json();
    window.dispatchEvent(new CustomEvent('navigate', { detail: { view: 'jobmonitor', jobId: d.id } }));
  }
</script>

<div class="p-6">
  <h2 class="text-lg font-semibold mb-4">ESPHome Compile Jobs</h2>

  {#if loading}
    <span class="loading loading-spinner loading-lg"></span>
  {:else if error}
    <div class="alert alert-error">{error}</div>
  {:else if jobs.length === 0}
    <div class="text-base-content/50 text-sm">No compile jobs yet.</div>
  {:else}
    <div class="overflow-x-auto">
      <table class="table table-sm w-full">
        <thead>
          <tr>
            <th>Device</th>
            <th>Status</th>
            <th>Created</th>
            <th class="text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each jobs as j}
            <tr class="hover cursor-pointer" on:click={() => openJob(j.id)}>
              <td class="font-medium">{j.device_name}</td>
              <td>
                <span class="badge badge-sm {statusClass[j.status] || 'badge-ghost'}">
                  {#if j.status === 'running'}<span class="loading loading-spinner loading-xs mr-1"></span>{/if}
                  {j.status}
                </span>
              </td>
              <td class="text-xs text-base-content/50">
                {new Date(j.created_at).toLocaleString()}
              </td>
              <td class="text-right space-x-1">
                {#if j.status === 'pending' || j.status === 'running'}
                  <button class="btn btn-xs btn-ghost" on:click={(e) => cancelJob(j.id, e)}>Cancel</button>
                {/if}
                <button class="btn btn-xs btn-ghost" on:click={(e) => resubmit(j.id, e)}>Re-compile</button>
                {#if j.status === 'done' && j.binary_path}
                  <a class="btn btn-xs btn-outline" href={`/api/jobs/${j.id}/firmware`}
                     download on:click={(e) => e.stopPropagation()}>Download</a>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
