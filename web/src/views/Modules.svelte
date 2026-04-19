<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import YamlModal from '../lib/YamlModal.svelte';

  let modules = [];
  let error = '';
  let loading = true;
  let filter = '';
  let categoryFilter = 'all';

  let modalOpen = false;
  let modalTitle = '';
  let modalYaml = '';

  let importOpen = false;
  let importYaml = '';
  let importError = '';

  onMount(async () => {
    try {
      modules = await api.get('/api/modules');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  $: filtered = modules.filter(m => {
    const matchName = (m.name || '').toLowerCase().includes(filter.toLowerCase()) ||
                      (m.id || '').toLowerCase().includes(filter.toLowerCase());
    const matchCat = categoryFilter === 'all' || m.category === categoryFilter;
    return matchName && matchCat;
  });

  function viewModule(m) {
    modalTitle = m.name;
    modalYaml = m.yaml_body;
    modalOpen = true;
  }

  async function importModule(yaml) {
    importError = '';
    try {
      const match = yaml.match(/^id:\s*(\S+)/m);
      if (!match) throw new Error('YAML must contain an "id:" field');
      const id = match[1];
      const nameMatch = yaml.match(/^name:\s*"?([^"\n]+)"?/m);
      const name = nameMatch ? nameMatch[1].replace(/"$/, '').trim() : id;
      const catMatch = yaml.match(/^category:\s*(\S+)/m);
      const category = catMatch ? catMatch[1] : '';
      await api.post('/api/modules', { id, name, category, yaml_body: yaml });
      modules = await api.get('/api/modules');
      importOpen = false;
      importYaml = '';
    } catch (e) {
      importError = e.message;
    }
  }

  const categoryBadge = c => ({
    driver: 'badge-primary',
    sensor: 'badge-secondary',
    io:     'badge-accent',
  }[c] || 'badge-ghost');
</script>

<YamlModal title={modalTitle} yaml={modalYaml} open={modalOpen} readonly
  onClose={() => modalOpen = false} />

<YamlModal title="Import Module YAML" bind:yaml={importYaml} open={importOpen}
  error={importError}
  onClose={() => { importOpen = false; importError = ''; }}
  onSave={importModule} />

<div class="p-6 flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold">Module Library</h2>
    <button class="btn btn-primary btn-sm" on:click={() => importOpen = true}>+ Import YAML</button>
  </div>

  <div class="flex gap-2 flex-wrap">
    <input
      class="input input-bordered input-sm flex-1 min-w-48"
      placeholder="Filter by name or ID…"
      bind:value={filter}
    />
    <select class="select select-bordered select-sm" bind:value={categoryFilter}>
      <option value="all">All categories</option>
      <option value="driver">Driver</option>
      <option value="sensor">Sensor</option>
      <option value="io">I/O</option>
    </select>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if filtered.length === 0}
    <div class="text-sm text-base-content/50 py-8 text-center">
      {modules.length === 0 ? 'No modules loaded.' : 'No modules match the filter.'}
    </div>
  {:else}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {#each filtered as m (m.id)}
        <button
          class="card bg-base-200 border border-base-300 hover:border-primary/50 transition-all text-left p-4"
          on:click={() => viewModule(m)}
        >
          <div class="flex items-start justify-between gap-2">
            <div>
              <div class="font-semibold text-sm">{m.name}</div>
              <div class="text-xs font-mono text-base-content/50 mt-0.5">{m.id}</div>
            </div>
            <span class="badge badge-sm {categoryBadge(m.category)} shrink-0">{m.category}</span>
          </div>
          {#if m.builtin}
            <div class="text-xs text-base-content/40 mt-2">Built-in</div>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
</div>
