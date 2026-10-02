<script lang="ts">
  import AudioAB from "../AudioAB.svelte";
  import { sideURL } from "./urls";
  import type { ViewerProps } from "./types";

  // A sample: its waveform to play; comparing, the other above it.
  let { root, a, b, compare, stamp }: ViewerProps = $props();
  const take = (s: typeof a) => (s ? { label: s.label, src: sideURL(root, s, stamp) } : null);
</script>

<AudioAB a={take(a)} b={compare ? take(b) : null} />
