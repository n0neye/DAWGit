<script lang="ts">
  import SetView from "../SetView.svelte";
  import type { ViewerProps } from "./types";

  // A Live Set: its tracks as Live's Arrangement or Session view shows them,
  // or as text; comparing, what changed in them.
  let { root, a, b, compare, stamp }: ViewerProps = $props();
</script>

<SetView {root} {stamp} {compare} file={(a ?? b)!.path} version={a ? a.version : "none"}
  fromFile={b?.path ?? ""} fromVersion={b ? b.version : "none"} />
