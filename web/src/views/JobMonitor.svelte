<script>
  import { onMount, onDestroy } from 'svelte';

  export let jobId = '';

  let job = null;
  let logs = [];
  let status = '';
  let position = null;
  let error = '';
  let loading = true;
  let es = null;

  onMount(async () => {
    try {
      const res = await fetch(`/api/jobs/${jobId}`);
      if (!res.ok) throw new Error(await res.text());
      job = await res.json();
      status = job.status;
      if (job.log) logs = job.log.split('\n').filter(Boolean);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }

    if (status !== 'done' && status !== 'failed' && status !== 'cancelled') {
      connectSSE();
    }
  });

  onDestroy(() => { if (es) es.close(); });

  function connectSSE() {
    es = new EventSource(`/api/jobs/${jobId}/stream`);
    es.onmessage = (e) => {
      const ev = JSON.parse(e.data);
      if (ev.type === 'log') {
        logs = [...logs, ev.data];
      } else if (ev.type === 'status') {
        status = ev.data;
      } else if (ev.type === 'position') {
        position = ev.data;
      } else if (ev.type === 'done') {
        status = ev.ok ? 'done' : 'failed';
        error = ev.error || '';
        es.close();
        fetch(`/api/jobs/${jobId}`).then(r => r.json()).then(j => { job = j; });
      }
    };
    es.onerror = () => {};
  }

  function recompile() {
    fetch(`/api/jobs/${jobId}/resubmit`, { method: 'POST' })
      .then(r => r.json())
      .then(d => {
        window.dispatchEvent(new CustomEvent('navigate', {
          detail: { view: 'jobmonitor', jobId: d.id }
        }));
      });
  }

  const statusClass = {
    pending:   'badge-warning',
    running:   'badge-info',
    done:      'badge-success',
    failed:    'badge-error',
    cancelled: 'badge-ghost',
  };
</script>

<div class="p-6 max-w-3xl mx-auto">
  {#if loading}
    <span class="loading loading-spinner loading-lg"></span>
  {:else if error && !job}
    <div class="alert alert-error">{error}</div>
  {:else}
    <div class="flex items-center gap-3 mb-4">
      <h2 class="text-lg font-semibold">{job?.device_name || jobId}</h2>
      <span class="badge {statusClass[status] || 'badge-ghost'}">{status}</span>
    </div>

    {#if status === 'pending'}
      <div class="alert alert-warning mb-4">
        <span class="loading loading-spinner loading-sm"></span>
        {#if position}Queued — position {position}{:else}Waiting in queue…{/if}
        <button class="btn btn-sm btn-ghost ml-auto"
          on:click={() => fetch(`/api/jobs/${jobId}`, { method: 'DELETE' }).then(() => { status = 'cancelled'; })}>
          Cancel
        </button>
      </div>
    {/if}

    {#if status === 'running'}
      <div class="flex items-center gap-2 mb-2 text-sm text-info">
        <span class="loading loading-spinner loading-xs"></span> Compiling…
        <button class="btn btn-xs btn-ghost ml-auto"
          on:click={() => fetch(`/api/jobs/${jobId}`, { method: 'DELETE' })}>
          Cancel
        </button>
      </div>
    {/if}

    {#if status === 'done'}
      <div class="alert alert-success mb-4">
        Firmware compiled successfully.
        {#if job?.binary_path}
          <a href={`/api/jobs/${jobId}/firmware`}
             class="btn btn-sm btn-outline ml-auto" download>Download .bin</a>
        {/if}
        <button class="btn btn-sm btn-outline" on:click={recompile}>Re-compile</button>
      </div>
    {/if}

    {#if status === 'failed'}
      <div class="alert alert-error mb-4">
        Compile failed: {error || job?.error || 'unknown error'}
        <button class="btn btn-sm btn-outline ml-auto" on:click={recompile}>Re-compile</button>
      </div>
    {/if}

    {#if status === 'cancelled'}
      <div class="alert mb-4">
        Cancelled.
        <button class="btn btn-sm btn-outline ml-auto" on:click={recompile}>Re-compile</button>
      </div>
    {/if}

    {#if logs.length > 0}
      <div class="bg-base-300 rounded-lg p-3 font-mono text-xs overflow-y-auto max-h-96 space-y-0.5">
        {#each logs as line}
          <div class="whitespace-pre-wrap break-all">{line}</div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
