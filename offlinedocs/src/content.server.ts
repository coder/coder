import { readFileSync } from "node:fs";
import path from "node:path";
import fm from "front-matter";
import sanitizeHtml from "sanitize-html";
import {
	type DocsPageData,
	getNavigation,
	type Manifest,
	mapRoutes,
	type Nav,
} from "./content";

const docsDir = () => path.join(process.cwd(), "..", "docs");

const readManifest = (): Manifest => {
	return JSON.parse(
		readFileSync(path.join(docsDir(), "manifest.json"), {
			encoding: "utf-8",
		}),
	) as Manifest;
};

export const getUrlPaths = (): string[] => {
	return Object.keys(mapRoutes(readManifest()));
};

export const getLayoutData = (): { navigation: Nav; version: string } => {
	const manifest = readManifest();
	return {
		navigation: getNavigation(manifest),
		version: manifest.versions[0],
	};
};

export const getPage = (urlPath: string): DocsPageData | undefined => {
	const route = mapRoutes(readManifest())[urlPath];
	if (!route) {
		return undefined;
	}

	const { attributes, body } = fm<{ title?: string }>(
		readFileSync(path.join(docsDir(), route.path), {
			encoding: "utf-8",
		}),
	);
	const title =
		typeof attributes.title === "string" && attributes.title.trim() !== ""
			? attributes.title
			: route.title;

	return {
		content: sanitizeHtml(body),
		route,
		title,
	};
};
