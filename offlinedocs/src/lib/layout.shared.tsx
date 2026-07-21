import type { BaseLayoutProps } from "fumadocs-ui/layouts/shared";
import { CoderMark } from "@/components/coder-mark";
import { appName, gitConfig } from "./shared";

export function baseOptions(): BaseLayoutProps {
	return {
		nav: {
			title: (
				<span className="inline-flex items-center gap-2 font-semibold">
					<CoderMark className="size-5 text-fd-foreground" />
					{appName}
				</span>
			),
		},
		githubUrl: `https://github.com/${gitConfig.user}/${gitConfig.repo}`,
	};
}
