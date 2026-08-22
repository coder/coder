import type { BaseLayoutProps } from "fumadocs-ui/layouts/shared";
import { CoderMark } from "@/components/coder-mark";

const appName = "Coder Docs";

// The corpus is rendered from the public coder/coder repository. Used for the
// GitHub link in the top navigation.
const githubRepo = "coder/coder";

export function baseOptions(): BaseLayoutProps {
	return {
		nav: {
			title: (
				<span className="inline-flex items-center gap-2 font-medium">
					<CoderMark className="size-5 text-fd-foreground" />
					{appName}
				</span>
			),
		},
		githubUrl: `https://github.com/${githubRepo}`,
	};
}
