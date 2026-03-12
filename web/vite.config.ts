import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
    plugins: [react(), tailwindcss()],
    base: "/ui/",
    resolve: {
        alias: {
            "@": path.resolve(__dirname, "./src"),
        },
    },
    build: {
        outDir: "dist",
        emptyOutDir: true,
    },
    server: {
        port: 5173,
        proxy: {
            "/api": {
                target: "http://localhost:8081",
                changeOrigin: true,
            },
            "/healthz": {
                target: "http://localhost:8081",
                changeOrigin: true,
            },
            "/metrics": {
                target: "http://localhost:8081",
                changeOrigin: true,
            },
        },
    },
});
