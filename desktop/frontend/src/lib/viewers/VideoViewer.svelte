<script lang="ts">
  import { api, errorText } from "../api";
  import { toast } from "../notify.svelte";
  import VideoCompare from "../VideoCompare.svelte";
  import { sideURL } from "./urls";
  import type { ViewerProps } from "./types";

  // A video to watch; comparing, the other beside it, played together.
  let { root, a, b, compare, stamp }: ViewerProps = $props();
  const take = (s: typeof a) => (s ? { label: s.label, src: sideURL(root, s, stamp) } : null);
  const shown = $derived(a ?? b);
</script>

<VideoCompare a={take(a)} b={take(b)} {compare}
  onopen={shown && !shown.version ? () => api.OpenInLive(root, shown.path).catch((e) => toast(errorText(e), "error")) : undefined} />
