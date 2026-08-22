import { source } from "@/lib/source";
import { DocsLayout } from "fumadocs-ui/layouts/docs";
import { baseOptions } from "@/lib/layout.shared";
import { Provider } from "@/components/provider";
// Self-hosted brand fonts, matching the Coder product (site/src/theme/
// globalFonts.ts). Variable fonts are bundled locally so the offline bundle
// needs no network.
import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "./global.css";

// Every route in this bundle is a docs page served at the site root, so the
// docs shell (sidebar + nav) lives in the root layout.
export default function Layout({ children }: LayoutProps<"/">) {
	return (
		<html lang="en" suppressHydrationWarning>
			<body className="flex flex-col min-h-screen">
				<Provider>
					<DocsLayout tree={source.pageTree} {...baseOptions()}>
						{children}
					</DocsLayout>
				</Provider>
			</body>
		</html>
	);
}
