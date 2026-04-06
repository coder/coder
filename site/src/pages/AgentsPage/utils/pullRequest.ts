const repoContentRoutePattern =
	/\/(?:tree|blob|compare|commit|commits|branches|releases|tags|wiki)\//;

export const parsePullRequestUrl = (
	url: string | null | undefined,
): { owner: string; repo: string; number: string } | null => {
	if (!url) {
		return null;
	}

	try {
		const { pathname } = new URL(url);
		const segments = pathname.split("/").filter(Boolean);
		if (segments.length < 4) {
			return null;
		}

		const pullSegmentIndex = segments.findIndex((segment, index) => {
			if (segment !== "pull") {
				return false;
			}

			const number = segments.at(index + 1);
			if (!number || !/^\d+$/.test(number)) {
				return false;
			}

			const leadingPath = `/${segments.slice(0, index).join("/")}/`;
			return !repoContentRoutePattern.test(leadingPath);
		});
		if (pullSegmentIndex < 2) {
			return null;
		}

		const number = segments.at(pullSegmentIndex + 1);
		if (!number) {
			return null;
		}

		return {
			owner: segments.at(pullSegmentIndex - 2) ?? "",
			repo: segments.at(pullSegmentIndex - 1) ?? "",
			number,
		};
	} catch {
		return null;
	}
};
