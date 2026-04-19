<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import YamlModal from '../lib/YamlModal.svelte';

  let effects = [];
  let error = '';
  let loading = true;

  let modalOpen = false;
  let modalTitle = '';
  let modalYaml = '';

  let importOpen = false;
  let importYaml = '';
  let importError = '';

  let deleteTarget = null;

  onMount(async () => {
    try {
      effects = await api.get('/api/effects');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  function viewEffect(e) {
    modalTitle = e.name || e.id;
    modalYaml = e.yaml_body;
    modalOpen = true;
  }

  async function importEffect(yaml) {
    importError = '';
    const idMatch = yaml.match(/^id:\s*(\S+)/m);
    if (!idMatch) { importError = 'YAML must contain an "id:" field'; return; }
    const id = idMatch[1];
    const nameMatch = yaml.match(/^name:\s*"?([^"\n]+)"?/m);
    const name = nameMatch ? nameMatch[1].replace(/"$/, '').trim() : id;
    try {
      await api.post('/api/effects', { id, name, yaml_body: yaml });
      effects = await api.get('/api/effects');
      importOpen = false;
      importYaml = '';
      importError = '';
    } catch (e) {
      importError = e.message;
    }
  }

  async function doDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/api/effects/${deleteTarget.id}`);
      effects = await api.get('/api/effects');
    } catch (e) {
      error = e.message;
    } finally {
      deleteTarget = null;
    }
  }
</script>

<YamlModal title={modalTitle} yaml={modalYaml} open={modalOpen} readonly
  onClose={() => modalOpen = false} />

<YamlModal title="Import Effect YAML" bind:yaml={importYaml} open={importOpen}
  error={importError}
  onClose={() => { importOpen = false; importError = ''; }}
  onSave={importEffect} />

{#if deleteTarget}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
    <div class="bg-base-200 rounded-xl p-6 shadow-xl max-w-sm w-full mx-4">
      <h3 class="font-semibold mb-2">Delete effect?</h3>
      <p class="text-sm text-base-content/70 mb-4">
        "<strong>{deleteTarget.name || deleteTarget.id}</strong>" will be permanently removed.
      </p>
      <div class="flex justify-end gap-2">
        <button class="btn btn-ghost btn-sm" on:click={() => deleteTarget = null}>Cancel</button>
        <button class="btn btn-error btn-sm" on:click={doDelete}>Delete</button>
      </div>
    </div>
  </div>
{/if}

<div class="p-6 flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold">Effects</h2>
    <button class="btn btn-primary btn-sm" on:click={() => importOpen = true}>+ Import YAML</button>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if effects.length === 0}
    <div class="text-sm text-base-content/50 py-8 text-center">No effects yet. Import a YAML to get started.</div>
  {:else}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {#each effects as e (e.id)}
        <div class="card bg-base-200 border border-base-300 p-4">
          <div class="flex items-start justify-between gap-2">
            <div>
              <div class="font-semibold text-sm">{e.name || e.id}</div>
              <div class="text-xs font-mono text-base-content/50 mt-0.5">{e.id}</div>
            </div>
            {#if e.builtin}
              <span class="badge badge-ghost badge-sm shrink-0">built-in</span>
            {/if}
          </div>
          <div class="flex gap-2 mt-3">
            <button class="btn btn-ghost btn-xs flex-1" on:click={() => viewEffect(e)}>View YAML</button>
            {#if !e.builtin}
              <button class="btn btn-error btn-xs" on:click={() => deleteTarget = e}>✕</button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
