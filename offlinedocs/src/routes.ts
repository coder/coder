import { index, type RouteConfig, route } from "@react-router/dev/routes";

export default [
	index("routes/page.tsx"),
	route("*", "routes/page.tsx", { id: "splat" }),
] satisfies RouteConfig;
