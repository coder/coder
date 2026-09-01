import { urlPathFromLocation } from "../content";
import { getLayoutData, getPage } from "../content.server";
import { DocsPage } from "../DocsPage";
import type { Route } from "./+types/page";

export const meta: Route.MetaFunction = ({ data }) => {
	if (!data) {
		return [{ title: "Coder Docs" }];
	}

	return [
		{ title: data.page.title },
		{ name: "source", content: data.page.route.path },
	];
};

export async function loader({ request }: Route.LoaderArgs) {
	const urlPath = urlPathFromLocation(new URL(request.url).pathname);
	const page = getPage(urlPath);
	if (!page) {
		throw new Response("Not Found", { status: 404 });
	}

	return { page, ...getLayoutData() };
}

export default function Page({ loaderData }: Route.ComponentProps) {
	return <DocsPage page={loaderData.page} navigation={loaderData.navigation} />;
}
