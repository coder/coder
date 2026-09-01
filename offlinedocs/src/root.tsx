import { ChakraProvider, extendTheme } from "@chakra-ui/react";
import type { ReactNode } from "react";
import {
	isRouteErrorResponse,
	Links,
	Meta,
	Outlet,
	Scripts,
	ScrollRestoration,
} from "react-router";
import type { Route } from "./+types/root";

const theme = extendTheme({
	styles: {
		global: {
			body: {
				bg: "gray.50",
			},
		},
	},
});

export const links: Route.LinksFunction = () => [
	{ rel: "mask-icon", href: "/favicon.svg", color: "#000000" },
	{ rel: "alternate icon", type: "image/png", href: "/favicon.png" },
];

export function Layout({ children }: { children: ReactNode }) {
	return (
		<html lang="en">
			<head>
				<meta charSet="utf-8" />
				<meta name="viewport" content="width=device-width, initial-scale=1" />
				<Meta />
				<Links />
			</head>
			<body>
				{children}
				<ScrollRestoration />
				<Scripts />
			</body>
		</html>
	);
}

export default function App() {
	return (
		<ChakraProvider theme={theme}>
			<Outlet />
		</ChakraProvider>
	);
}

export function HydrateFallback() {
	return null;
}

export function ErrorBoundary({ error }: Route.ErrorBoundaryProps) {
	let message = "Error";
	let details = "An unexpected error occurred.";

	if (isRouteErrorResponse(error)) {
		message = error.status === 404 ? "404" : "Error";
		details =
			error.status === 404
				? "The requested page could not be found."
				: error.statusText || details;
	} else if (import.meta.env.DEV && error instanceof Error) {
		details = error.message;
	}

	return (
		<main>
			<h1>{message}</h1>
			<p>{details}</p>
		</main>
	);
}
