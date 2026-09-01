import { reactRouter } from "@react-router/dev/vite";
import { defineConfig } from "vite";

export default defineConfig({
	plugins: [reactRouter()],
	server: {
		port: 3000,
		fs: {
			allow: [".."],
		},
	},
});
