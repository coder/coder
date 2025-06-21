import GitHub from "@mui/icons-material/GitHub";
import { Button } from "components/Button/Button";
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
							label = number
								? `${org}/${repo}#${number}`
								: `${org}/${repo} pull request`;
							break;
						case "issues":
							icon = <BugIcon />;
							label = number
								? `${org}/${repo}#${number}`
								: `${org}/${repo} issue`;
							break;
						default:
							icon = <GitHub />;
							if (org && repo) {
								label = `${org}/${repo}`;
							}
							break;
					}
				}
				break;
		}
	} catch (error) {
		// Invalid URL, probably.
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
