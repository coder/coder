import Link from "@mui/material/Link";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink, useParams } from "react-router";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { Markdown } from "#/components/Markdown/Markdown";
import { getStaticBuildInfo } from "#/utils/buildInfo";
import { pageTitle } from "#/utils/page";
import { DocsSidebar } from "./DocsSidebar";
import {
	buildRouteMaps,
	type DocsManifest,
	type DocsRouteMaps,
	docsManifestQuery,
	docsPageQuery,
	githubDocsAssetUrl,
	resolveRelativeDocLink,
} from "./docsContent";

const DocsPage: FC = () => {
	const params = useParams();
	const activeRoute = (params["*"] ?? "").replace(/\/+$/, "");
	const manifestQuery = useQuery(docsManifestQuery());

	if (manifestQuery.error) {
		return <ErrorAlert error={manifestQuery.error} />;
	}
	if (!manifestQuery.data) {
		return <Loader fullscreen />;
	}

	return <DocsLayout manifest={manifestQuery.data} activeRoute={activeRoute} />;
};

interface DocsLayoutProps {
	manifest: DocsManifest;
	activeRoute: string;
}

const DocsLayout: FC<DocsLayoutProps> = ({ manifest, activeRoute }) => {
	const { routeToFile, fileToRoute, routeToTitle } = buildRouteMaps(manifest);
	const filePath = routeToFile.get(activeRoute);
	const activeTitle = routeToTitle.get(activeRoute);

	return (
		<>
			<title>
				{activeTitle ? pageTitle(`${activeTitle} - Docs`) : pageTitle("Docs")}
			</title>
			<div className="mx-auto flex w-full max-w-screen-xl grow gap-8 px-6">
				<DocsSidebar routes={manifest.routes} activeRoute={activeRoute} />
				{filePath === undefined ? (
					<div className="py-10">
						<h1>Page not found</h1>
						<p>
							This page does not exist in this version of the documentation.{" "}
							<RouterLink to="/docs">Back to the docs home page.</RouterLink>
						</p>
					</div>
				) : (
					<DocsContent filePath={filePath} fileToRoute={fileToRoute} />
				)}
			</div>
		</>
	);
};

interface DocsContentProps {
	filePath: string;
	fileToRoute: DocsRouteMaps["fileToRoute"];
}

const DocsContent: FC<DocsContentProps> = ({ filePath, fileToRoute }) => {
	const pageQuery = useQuery(docsPageQuery(filePath));
	const version = getStaticBuildInfo()?.version;

	return (
		<main className="min-w-0 grow overflow-x-hidden py-8">
			{pageQuery.error ? (
				<ErrorAlert error={pageQuery.error} />
			) : pageQuery.data === undefined ? (
				<Loader />
			) : (
				<Markdown
					components={{
						a: ({ href = "", children }) => {
							const resolved = resolveRelativeDocLink(filePath, href);
							if (resolved) {
								const route = fileToRoute.get(resolved.path);
								if (route !== undefined) {
									return (
										<Link
											component={RouterLink}
											to={`/docs/${route}${resolved.hash}`}
										>
											{children}
										</Link>
									);
								}
								// Relative link to a repo file outside the docs manifest,
								// for example linked example code. Send it to GitHub.
								return (
									<Link
										href={githubDocsAssetUrl(resolved.path, version)}
										target="_blank"
										rel="noreferrer"
									>
										{children}
									</Link>
								);
							}
							const isExternal =
								href.startsWith("http") || href.startsWith("//");
							return (
								<Link
									href={href}
									target={isExternal ? "_blank" : undefined}
									rel={isExternal ? "noreferrer" : undefined}
								>
									{children}
								</Link>
							);
						},
						img: ({ src = "", alt }) => {
							const srcString = typeof src === "string" ? src : "";
							const resolved = resolveRelativeDocLink(filePath, srcString);
							return (
								<img
									src={
										resolved
											? githubDocsAssetUrl(resolved.path, version)
											: srcString
									}
									alt={alt ?? ""}
									className="max-w-full"
								/>
							);
						},
					}}
				>
					{pageQuery.data}
				</Markdown>
			)}
		</main>
	);
};

export default DocsPage;
