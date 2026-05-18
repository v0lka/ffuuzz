import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

export default mergeConfig(
    viteConfig,
    defineConfig({
        test: {
            environment: "jsdom",
            globals: true,
            setupFiles: ["./src/test/setup.ts"],
            include: ["src/**/*.{test,spec}.{ts,tsx}"],
            css: false,
            coverage: {
                provider: "v8",
                reporter: ["text", "json", "html", "lcov"],
                include: ["src/**/*.{ts,tsx}"],
                exclude: [
                    "src/main.tsx",
                    "src/vite-env.d.ts",
                    "src/test/**",
                    "src/**/*.test.{ts,tsx}",
                    "src/**/*.spec.{ts,tsx}",
                ],
            },
        },
    }),
);
