<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import YamlModal from '../lib/YamlModal.svelte';

  let templates = [];
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
      templates = await api.get('/api/templates');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  function viewTemplate(t) {
    modalTitle = t.name || t.id;
    modalYaml = t.yaml_body;
    modalOpen = true;
  }

  async function importTemplate(yaml) {
    importError = '';
    const idMatch = yaml.match(/^id:\s*(\S+)/m);
    if (!idMatch) { importError = 'YAML must contain an "id:" field'; return; }
    const id = idMatch[1];
    const nameMatch = yaml.match(/^name:\s*"?([^"\n]+)"?/m);
    const name = nameMatch ? nameMatch[1].replace(/"$/, '').trim() : id;
    try {
      await api.post('/api/templates', { id, name, yaml_body: yaml });
      templates = await api.get('/api/templates');
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
      await api.delete(`/api/templates/${deleteTarget.id}`);
      templates = await api.get('/api/templates');
    } catch (e) {
      error = e.message;
    } finally {
      deleteTarget = null;
    }
  }
</script>

<YamlModal title={modalTitle} yaml={modalYaml} open={modalOpen} readonly
  onClose={() => modalOpen = false} />

<YamlModal title="Import Template YAML" bind:yaml={importYaml} open={importOpen}
  error={importError}
  onClose={() => { importOpen = false; importError = ''; }}
  onSave={importTemplate} />

{#if deleteTarget}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
    <div class="bg-base-200 rounded-xl p-6 shadow-xl max-w-sm w-full mx-4">
      <h3 class="font-semibold mb-2">Delete template?</h3>
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
    <h2 class="text-lg font-semibold">Templates</h2>
    <button class="btn btn-primary btn-sm" on:click={() => importOpen = true}>+ Import YAML</button>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if templates.length === 0}
    <div class="text-sm text-base-content/50 py-8 text-center">
      No templates yet. Import a YAML to get started.
    </div>
  {:else}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {#each templates as t (t.id)}
        <div class="card bg-base-200 border border-base-300 p-4">
          <div class="font-semibold text-sm">{t.name || t.id}</div>
          <div class="text-xs font-mono text-base-content/50 mt-0.5">{t.id}</div>
          <div class="text-xs text-base-content/40 mt-1">Board: {t.board}</div>
          <div class="flex gap-2 mt-3">
            <button class="btn btn-ghost btn-xs flex-1" on:click={() => viewTemplate(t)}>View YAML</button>
            <button class="btn btn-error btn-xs" on:click={() => deleteTarget = t}>✕</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
