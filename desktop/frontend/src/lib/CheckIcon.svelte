<script lang="ts">
  // Line icons for the project check (24-unit grid, drawn with the text color).
  let { name, size = 18 }: { name: string; size?: number } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"
  stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  {#if name === "live"}
    <rect x="3" y="4" width="18" height="16" rx="3" /><path d="M10 9l5 3-5 3z" />
  {:else if name === "wave"}
    <path d="M3 12h2M7 8v8M11 5v14M15 9v6M19 7v10M21 12h0" />
  {:else if name === "plug"}
    <path d="M9 3v5M15 3v5M6 8h12v3a6 6 0 0 1-12 0zM12 17v4" />
  {:else if name === "package"}
    <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z" /><path d="M4 7.5l8 4.5 8-4.5M12 12v9" />
  {:else if name === "users"}
    <circle cx="9" cy="8" r="3.5" /><path d="M2.5 20a6.5 6.5 0 0 1 13 0" /><path d="M16 4.6a3.5 3.5 0 0 1 0 6.8M18 14.2a6.5 6.5 0 0 1 3.5 5.8" />
  {:else if name === "upload"}
    <path d="M7 18a5 5 0 0 1-.8-9.9A6 6 0 0 1 17.6 7 4.5 4.5 0 0 1 17 18" /><path d="M12 12v8M9 15l3-3 3 3" />
  {:else if name === "folder"}
    <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
  {:else if name === "link"}
    <path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5" />
  {:else if name === "check"}
    <path d="M5 12.5l4.5 4.5L19 7.5" />
  {:else if name === "alert"}
    <path d="M12 3l9.5 17h-19z" /><path d="M12 10v4M12 17.5h0" />
  {:else if name === "x"}
    <path d="M6 6l12 12M18 6L6 18" />
  {:else if name === "question"}
    <circle cx="12" cy="12" r="9" /><path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .9-1 1.6v.6M12 17h0" />
  {/if}
</svg>
