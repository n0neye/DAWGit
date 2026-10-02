<script lang="ts">
  // A translated sentence with some of its {names} in bold, italic or code:
  // <Tx text={t("Switching to {version} didn't finish")} strong={{ version: "“Mix 2”" }} />
  // (the sentence is translated whole; word order differs by language).
  let { text, strong = {}, em = {}, code = {}, vars = {} }: {
    text: string;
    strong?: Record<string, string | number>;
    em?: Record<string, string | number>;
    code?: Record<string, string | number>;
    vars?: Record<string, string | number>;
  } = $props();

  let parts = $derived(text.split(/(\{\w+\})/).map((p) => {
    const k = p.match(/^\{(\w+)\}$/)?.[1];
    if (k !== undefined) {
      for (const [kind, map] of [["strong", strong], ["em", em], ["code", code], ["", vars]] as const) {
        if (k in map) return { kind, text: String(map[k]) };
      }
    }
    return { kind: "", text: p };
  }));
</script>

{#each parts as p}{#if p.kind === "strong"}<strong>{p.text}</strong>{:else if p.kind === "em"}<em>{p.text}</em>{:else if p.kind === "code"}<span class="mono">{p.text}</span>{:else}{p.text}{/if}{/each}
