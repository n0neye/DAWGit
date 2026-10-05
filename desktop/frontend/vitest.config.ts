import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Unit and component tests (npm test): logic in .ts files, and components
// rendered in jsdom with the Go bindings mocked (vi.mock("./api")).
export default defineConfig({
  plugins: [svelte()],
  resolve: { conditions: ["browser"] },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts"],
  },
});
