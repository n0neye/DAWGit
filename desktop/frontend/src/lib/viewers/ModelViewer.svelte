<script lang="ts">
  import { fileURL } from "../api";
  import ModelCompare from "../ModelCompare.svelte";
  import type { Side, ViewerProps } from "./types";

  // A 3D model in a viewer; comparing, the other beside it, cameras together.
  let { root, file, a, b, compare }: ViewerProps = $props();

  const extOf = (p: string) => p.slice(p.lastIndexOf(".")).toLowerCase();
  // The model and the files it names (a glTF's .bin and textures) from the
  // same version. Now's address changes with the file's size, not with every
  // reload of the list: the viewer (and where you turned it) stays.
  function take(s: Side | null) {
    if (!s) return null;
    const dir = s.path.includes("/") ? s.path.slice(0, s.path.lastIndexOf("/") + 1) : "";
    const src = s.version ? fileURL(root, s.path, s.version) : `${fileURL(root, s.path)}&size=${file.size}`;
    return { label: s.label, src, resolve: (rel: string) => fileURL(root, dir + rel, s.version) };
  }
  const key = $derived(`${a?.path}@${a?.version}|${b?.path}@${b?.version}`);
</script>

{#key key}
  <ModelCompare ext={extOf((a ?? b)!.path)} a={take(a)} b={take(b)} {compare} />
{/key}
