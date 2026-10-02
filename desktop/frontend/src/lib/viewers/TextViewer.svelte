<script lang="ts">
  import TextView from "../TextView.svelte";
  import type { ViewerProps } from "./types";

  // Text and code: the whole file; comparing, its changes line by line.
  let { root, a, b, compare, stamp }: ViewerProps = $props();
</script>

<TextView {root} {stamp} {compare} file={(a ?? b)!.path} fromFile={b?.path ?? ""}
  from={b ? b.version : "none"} to={a ? a.version : "none"} />
