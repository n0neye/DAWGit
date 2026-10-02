<script lang="ts">
  import ImageCompare from "../ImageCompare.svelte";
  import { sidePreviewURL } from "./urls";
  import type { ViewerProps } from "./types";

  // A picture or design file; comparing, both large, side by side or under a
  // slider (its own setting).
  let { root, a, b, compare, stamp }: ViewerProps = $props();
  const take = (s: typeof a) => (s ? { label: s.label, src: sidePreviewURL(root, s, stamp) } : null);
</script>

<ImageCompare a={take(a)} b={take(b)} {compare} />
