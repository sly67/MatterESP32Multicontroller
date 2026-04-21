<script>
  import { onMount, onDestroy } from 'svelte';
  import Sidebar    from './lib/Sidebar.svelte';
  import Fleet      from './views/Fleet.svelte';
  import Flash      from './views/Flash.svelte';
  import Templates  from './views/Templates.svelte';
  import Modules    from './views/Modules.svelte';
  import Effects    from './views/Effects.svelte';
  import OTA        from './views/OTA.svelte';
  import Firmware   from './views/Firmware.svelte';
  import Settings   from './views/Settings.svelte';
  import Jobs       from './views/Jobs.svelte';
  import JobMonitor from './views/JobMonitor.svelte';

  let current = 'fleet';
  let jobMonitorId = '';

  let handleNavigate;
  onMount(() => {
    handleNavigate = (e) => {
      const { view, jobId } = e.detail;
      if (view === 'jobmonitor' && jobId) {
        jobMonitorId = jobId;
        current = 'jobmonitor';
      } else {
        current = view;
      }
    };
    window.addEventListener('navigate', handleNavigate);
  });
  onDestroy(() => {
    if (handleNavigate) window.removeEventListener('navigate', handleNavigate);
  });

  const plainViews = {
    fleet: Fleet, flash: Flash, templates: Templates, modules: Modules,
    effects: Effects, ota: OTA, firmware: Firmware, settings: Settings, jobs: Jobs
  };
  $: isJobMonitor = current === 'jobmonitor';
  $: ViewComponent = plainViews[current];
</script>

<div class="flex h-screen w-screen overflow-hidden bg-base-100 text-base-content">
  <Sidebar bind:current />
  <main class="flex-1 flex flex-col overflow-hidden">
    <div class="navbar bg-base-200 border-b border-base-200 px-4 min-h-12 flex-shrink-0">
      <span class="font-semibold text-sm capitalize">
        {isJobMonitor ? 'Job Monitor' : current}
      </span>
    </div>
    <div class="flex-1 overflow-y-auto">
      {#if isJobMonitor}
        <JobMonitor jobId={jobMonitorId} />
      {:else if ViewComponent}
        <svelte:component this={ViewComponent} />
      {/if}
    </div>
  </main>
</div>
