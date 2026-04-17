<script>
  import { onMount } from 'svelte';

  let versions = [];
  let error = '';
  let loading = true;
  let uploading = false;
  let uploadError = '';

  let version = '';
  let boards = '';
  let notes = '';
  let file = null;

  onMount(async () => { await load(); });

  async function load() {
    loading = true;
    try {
      const res = await fetch('/api/firmware');
      versions = await res.json();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function upload() {
    if (!version || !boards || !file) { uploadError = 'Version, boards and file are required.'; return; }
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
    await fetch(`/api/firmware/${v}/set-latest`, { method: 'POST' });
    await load();
  }

  async function remove(v) {
    await fetch(`/api/firmware/${v}`, { method: 'DELETE' });
    await load();
  }
</script>

<div class="p-6 flex flex-col gap-6 max-w-2xl">
  <h2 class="text-lg font-semibold">Firmware</h2>

  <div class="card bg-base-200 border border-base-300 p-4 flex flex-col gap-3">
    <div class="text-sm font-semibold">Upload New Firmware</div>
    {#if uploadError}<div class="alert alert-error text-xs">{uploadError}</div>{/if}
    <div class="grid grid-cols-2 gap-2">
      <input class="input input-bordered input-sm" placeholder="Version (e.g. 1.0.0)" bind:value={version} />
      <input class="input input-bordered input-sm" placeholder="Boards (e.g. esp32-c3,esp32-h2)" bind:value={boards} />
    </div>
    <input class="input input-bordered input-sm" placeholder="Release notes (optional)" bind:value={notes} />
    <input type="file" accept=".bin" class="file-input file-input-bordered file-input-sm"
      on:change={e => file = e.target.files[0]} />
    <button class="btn btn-primary btn-sm self-start" on:click={upload} disabled={uploading}>
      {uploading ? 'Uploading…' : 'Upload'}
    </button>
  </div>

  {#if loading}
    <div class="flex justify-center py-8"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if versions.length === 0}
    <div class="text-sm text-base-content/50 text-center py-6">No firmware uploaded yet.</div>
  {:else}
    <div class="overflow-x-auto rounded-lg border border-base-200">
      <table class="table table-sm">
        <thead><tr><th>Version</th><th>Boards</th><th>Notes</th><th>Status</th><th></th></tr></thead>
        <tbody>
          {#each versions as fw (fw.version)}
            <tr class="hover">
              <td class="font-mono text-sm">{fw.version}</td>
              <td class="text-xs">{fw.boards}</td>
              <td class="text-xs text-base-content/60">{fw.notes || '—'}</td>
              <td>{#if fw.is_latest}<span class="badge badge-success badge-sm">latest</span>{/if}</td>
              <td class="flex gap-1 justify-end">
                {#if !fw.is_latest}
                  <button class="btn btn-ghost btn-xs" on:click={() => setLatest(fw.version)}>Set latest</button>
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
