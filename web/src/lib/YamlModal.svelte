<script>
  export let title = '';
  export let yaml = '';
  export let open = false;
  export let readonly = false;
  export let error = '';   // caller sets this to show an error inside the modal

  export let onClose = () => {};
  export let onSave  = null;

  async function handleSave() {
    try { await onSave(yaml); } catch (_) { /* caller sets error prop */ }
  }
</script>

{#if open}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" aria-label="Close modal" on:click={onClose}></button>

  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-2xl flex flex-col max-h-[80vh]">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">{title}</span>
        <button class="btn btn-ghost btn-xs" on:click={onClose}>✕</button>
      </div>
      <div class="flex-1 overflow-auto p-4">
        <textarea
          class="textarea textarea-bordered font-mono text-xs w-full h-full min-h-64 resize-none"
          {readonly}
          bind:value={yaml}
        ></textarea>
      </div>
      {#if onSave}
        <div class="px-5 py-3 border-t border-base-300 flex flex-col gap-2">
          {#if error}
            <div class="alert alert-error text-xs py-2">{error}</div>
          {/if}
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-sm" on:click={onClose}>Cancel</button>
            <button class="btn btn-primary btn-sm" on:click={handleSave}>Save</button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}
