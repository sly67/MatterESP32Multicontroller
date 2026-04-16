<script>
  export let current = 'fleet';

  const nav = [
    { section: 'Overview' },
    { id: 'fleet',     label: 'Fleet',          icon: '⊞' },
    { id: 'flash',     label: 'Flash Devices',  icon: '⚡' },
    { section: 'Configuration' },
    { id: 'templates', label: 'Templates',      icon: '⚙' },
    { id: 'modules',   label: 'Module Library', icon: '⬡' },
    { section: 'Updates' },
    { id: 'ota',       label: 'OTA Updates',    icon: '⇅' },
    { id: 'firmware',  label: 'Firmware',       icon: '💾' },
    { section: 'System' },
    { id: 'settings',  label: 'Settings',       icon: '⚙' },
  ];
</script>

<aside class="w-56 flex-shrink-0 bg-base-300 border-r border-base-200 flex flex-col h-full p-3">
  <div class="flex items-center gap-2 px-2 py-3 mb-2 border-b border-base-200">
    <span class="text-lg">⚡</span>
    <div>
      <div class="font-bold text-sm text-white">MatterESP32</div>
      <div class="text-xs text-base-content/40">Hub</div>
    </div>
  </div>

  {#each nav as item}
    {#if item.section}
      <div class="text-xs uppercase tracking-widest text-base-content/40 px-3 pt-3 pb-1">
        {item.section}
      </div>
    {:else}
      <button
        class="flex items-center gap-2 px-3 py-2 rounded-lg text-sm w-full text-left transition-all
               {current === item.id
                 ? 'bg-primary/15 text-primary font-semibold'
                 : 'text-base-content/70 hover:bg-white/5 hover:text-white'}"
        on:click={() => current = item.id}
      >
        <span>{item.icon}</span>
        {item.label}
      </button>
    {/if}
  {/each}
</aside>
