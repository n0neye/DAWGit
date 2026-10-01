<script lang="ts">
  // A small icon per kind of file, so sets, samples, MIDI and the rest are
  // told apart at a glance.
  let { kind, open = false, faint = false }: {
    kind: string; // set | live | audio | midi | other | folder, and kinds presets add (scene, script…)
    open?: boolean; // folders
    faint?: boolean;
  } = $props();

  const titles: Record<string, string> = {
    set: "Live Set", live: "Live clip, preset or rack", audio: "Audio", midi: "MIDI", other: "File", folder: "Folder",
    scene: "Scene", level: "Level", prefab: "Prefab", asset: "Asset", script: "Code", image: "Image", model: "3D model",
    meta: "Unity .meta",
  };
</script>

<svg class="icon {kind}" class:faint viewBox="0 0 16 16" aria-hidden="true">
  <title>{titles[kind] ?? "File"}</title>
  {#if kind === "set"}
    <!-- a set: arrangement lanes -->
    <rect x="1.5" y="2" width="13" height="12" rx="2.5" class="fill" />
    <rect x="4" y="5" width="6" height="1.6" rx=".8" class="ink" />
    <rect x="6" y="7.2" width="6" height="1.6" rx=".8" class="ink" />
    <rect x="4" y="9.4" width="4.5" height="1.6" rx=".8" class="ink" />
  {:else if kind === "live"}
    <!-- a device / preset: a knob in a box -->
    <rect x="2" y="2" width="12" height="12" rx="2.5" class="line" />
    <circle cx="8" cy="8" r="3" class="line" />
    <path d="M8 8 L10 6" class="line" stroke-linecap="round" />
  {:else if kind === "audio"}
    <!-- a waveform -->
    <g class="bars">
      <rect x="1.5" y="6.5" width="1.6" height="3" rx=".8" />
      <rect x="4.2" y="4" width="1.6" height="8" rx=".8" />
      <rect x="6.9" y="1.8" width="1.6" height="12.4" rx=".8" />
      <rect x="9.6" y="4.8" width="1.6" height="6.4" rx=".8" />
      <rect x="12.3" y="6" width="1.6" height="4" rx=".8" />
    </g>
  {:else if kind === "midi"}
    <!-- piano keys -->
    <rect x="1.5" y="2.5" width="13" height="11" rx="2" class="line" />
    <path d="M5.8 2.5 V13.5 M10.2 2.5 V13.5" class="line" />
    <rect x="4.6" y="2.5" width="2.4" height="6" rx=".6" class="solid" />
    <rect x="9" y="2.5" width="2.4" height="6" rx=".6" class="solid" />
  {:else if kind === "folder"}
    {#if open}
      <path d="M1.5 4.5 a1.5 1.5 0 0 1 1.5 -1.5 h3 l1.5 1.5 h5 a1.5 1.5 0 0 1 1.5 1.5 v1 H4 L1.5 12.5 Z" class="solid" />
      <path d="M4 7 H15 L12.5 13 H1.5 Z" class="solid soft" />
    {:else}
      <path d="M1.5 4.5 a1.5 1.5 0 0 1 1.5 -1.5 h3 l1.5 1.5 h5 a1.5 1.5 0 0 1 1.5 1.5 v6 a1.5 1.5 0 0 1 -1.5 1.5 h-9.5 a1.5 1.5 0 0 1 -1.5 -1.5 Z" class="solid" />
    {/if}
  {:else if kind === "scene" || kind === "level"}
    <!-- a landscape: hills under a sun -->
    <rect x="1.5" y="2.5" width="13" height="11" rx="2" class="line" />
    <path d="M2.5 12 L6.5 7.5 L9 10 L11 8 L13.5 11" class="line" stroke-linejoin="round" />
    <circle cx="11" cy="5.5" r="1.2" class="solid" />
  {:else if kind === "prefab" || kind === "asset"}
    <!-- a cube -->
    <path d="M8 1.8 L14 5 V11 L8 14.2 L2 11 V5 Z" class="line" stroke-linejoin="round" />
    <path d="M2 5 L8 8.2 L14 5 M8 8.2 V14.2" class="line" stroke-linejoin="round" />
  {:else if kind === "script"}
    <!-- brackets -->
    <path d="M5.5 4 L2 8 L5.5 12 M10.5 4 L14 8 L10.5 12" class="line" stroke-linecap="round" stroke-linejoin="round" />
  {:else if kind === "image"}
    <rect x="1.5" y="2.5" width="13" height="11" rx="2" class="line" />
    <circle cx="5.5" cy="6.2" r="1.3" class="solid" />
    <path d="M2 12.5 L6.5 8.5 L9.5 11 L11.5 9.5 L14 11.5" class="line" stroke-linejoin="round" />
  {:else if kind === "model"}
    <!-- a wireframe sphere -->
    <circle cx="8" cy="8" r="6" class="line" />
    <ellipse cx="8" cy="8" rx="2.6" ry="6" class="line" />
    <path d="M2 8 H14" class="line" />
  {:else if kind === "meta"}
    <!-- a tag -->
    <path d="M2 3.5 a1.5 1.5 0 0 1 1.5 -1.5 h4 l6.5 6.5 l-5.5 5.5 l-6.5 -6.5 Z" class="line" stroke-linejoin="round" />
    <circle cx="5" cy="5" r="1" class="solid" />
  {:else}
    <!-- any other file -->
    <path d="M4 1.5 h5 l3.5 3.5 v8 a1.5 1.5 0 0 1 -1.5 1.5 h-7 a1.5 1.5 0 0 1 -1.5 -1.5 v-10 a1.5 1.5 0 0 1 1.5 -1.5 Z" class="line" />
    <path d="M9 1.5 v3.5 h3.5" class="line" />
  {/if}
</svg>

<style>
  .icon { width: 16px; height: 16px; flex: none; }
  .fill { fill: currentColor; }
  .ink { fill: var(--bg, #18191d); }
  .line { fill: none; stroke: currentColor; stroke-width: 1.3; }
  .solid { fill: currentColor; }
  .soft { opacity: .7; }
  .bars rect { fill: currentColor; }
  .set { color: #f0b44c; }
  .live { color: #e59b5f; }
  .audio { color: var(--accent); }
  .midi { color: #a78bfa; }
  .folder { color: #7c8490; }
  .other { color: var(--muted); }
  .scene, .level { color: #6ab0f3; }
  .prefab, .asset { color: #7fd1c7; }
  .script { color: #c3a6ff; }
  .image { color: #f29fc5; }
  .model { color: #f0b44c; }
  .meta { color: var(--faint); }
  .faint { opacity: .45; }
</style>
