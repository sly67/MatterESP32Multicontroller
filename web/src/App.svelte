<script>
  import Sidebar from './lib/Sidebar.svelte';
  import Fleet     from './views/Fleet.svelte';
  import Flash     from './views/Flash.svelte';
  import Templates from './views/Templates.svelte';
  import Modules   from './views/Modules.svelte';
  import OTA       from './views/OTA.svelte';
  import Firmware  from './views/Firmware.svelte';
  import Settings  from './views/Settings.svelte';

  let current = 'fleet';

  const views = { Fleet, Flash, Templates, Modules, OTA, Firmware, Settings };
  $: ViewComponent = views[current.charAt(0).toUpperCase() + current.slice(1)];
</script>

<div class="flex h-screen w-screen overflow-hidden bg-base-100 text-base-content">
  <Sidebar bind:current />
  <main class="flex-1 flex flex-col overflow-hidden">
    <div class="navbar bg-base-200 border-b border-base-200 px-4 min-h-12 flex-shrink-0">
      <span class="font-semibold text-sm capitalize">{current}</span>
    </div>
    <div class="flex-1 overflow-y-auto">
      <svelte:component this={ViewComponent} />
    </div>
  </main>
</div>
