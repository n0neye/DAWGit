// Live's clip and track colors, by the index a set stores (Live 10 on).
const palette = [
  "#FF94A6", "#FFA529", "#CC9927", "#F7F47C", "#BFFB00", "#1AFF2F", "#25FFA8", "#5CFFE8", "#8BC5FF", "#5480E4",
  "#92A7FF", "#D86CE4", "#E553A0", "#FFFFFF", "#FF3636", "#F66C03", "#99724B", "#FFF034", "#87FF67", "#3DC300",
  "#00BFAF", "#19E9FF", "#10A4EE", "#007DC0", "#886CE4", "#B677C6", "#FF39D4", "#D0D0D0", "#E2675A", "#FFA374",
  "#D3AD71", "#EDFFAE", "#D2E498", "#BAD074", "#9BC48D", "#D4FDE1", "#CDF1F8", "#B9C1E3", "#CDBBE4", "#AE98E5",
  "#E5DCE1", "#A9A9A9", "#C6928B", "#B78256", "#99836A", "#BFBA69", "#A6BE00", "#7DB04D", "#88C2BA", "#9BB3C4",
  "#85A5C2", "#8393CC", "#A595B5", "#BF9FBE", "#BC7196", "#7B7B7B", "#AF3333", "#A95131", "#724F41", "#DBC300",
  "#85961F", "#539F31", "#0A9C8E", "#236384", "#1A2F96", "#2F52A2", "#624BAD", "#A34BAD", "#CC2E6E", "#3C3C3C",
];

export const liveColor = (i: number) => palette[i] ?? "#8a8a8a";

// Dark text on light colors, light text on dark ones.
export function inkOn(i: number): string {
  const c = liveColor(i);
  const [r, g, b] = [1, 3, 5].map((k) => parseInt(c.slice(k, k + 2), 16));
  return 0.299 * r + 0.587 * g + 0.114 * b > 140 ? "#111" : "#f2f2f2";
}
