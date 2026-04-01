import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import {
	BugIcon,
	ExternalLinkIcon,
	GitPullRequestArrowIcon,
} from "lucide-react";
import type { FC } from "react";

type TaskStatusLinkProps = {
	uri: string;
};

export const TaskStatusLink: FC<TaskStatusLinkProps> = ({ uri }) => {
	let icon = <ExternalLinkIcon />;
	let label = uri;

	try {
		const parsed = new URL(uri);
		switch (parsed.protocol) {
			// For file URIs, strip off the `file://`.
			case "file:":
				label = uri.replace(/^file:\/\//, "");
				break;
			case "http:":
			case "https:":
				// For GitHub URIs, use a short representation.
				if (parsed.host === "github.com") {
					const [_, org, repo, type, number] = parsed.pathname.split("/");
					switch (type) {
						case "pull":
							icon = <GitPullRequestArrowIcon />;
							label =
								number === "new"
									? `${org}/${repo} open pull request`
									: number
										? `${org}/${repo}#${number}`
										: `${org}/${repo} pull request`;
							break;
						case "issues":
							icon = <BugIcon />;
							label =
								number === "new"
									? `${org}/${repo} create new issue`
									: number
										? `${org}/${repo}#${number}`
										: `${org}/${repo} issue`;
							break;
						default:
							icon = <ExternalImage src="/icon/github.svg" />;
							if (org && repo) {
								label = `${org}/${repo}`;
							}
							break;
					}
				}
				break;
		}
	} catch (_error) {
		// Invalid URL, probably.
		return null;
	}

	return (
		<Button asChild variant="outline" size="sm" className="min-w-0">
			<a href={uri} target="_blank" rel="noreferrer">
				{icon}
				<span className="truncate">{label}</span>
			</a>
		</Button>
	);
};
