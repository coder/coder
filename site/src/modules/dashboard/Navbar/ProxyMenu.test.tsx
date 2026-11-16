import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act, useState } from "react";
import { getPreferredProxy } from "#/contexts/ProxyContext";
import {
	MockProxyLatencies,
	MockWorkspaceProxies,
} from "#/testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { ProxyMenu } from "./ProxyMenu";

let finishRefresh: () => void = () => {};

const ProxyMenuWithRefresh: React.FC = () => {
	const [latencies, setLatencies] = useState(MockProxyLatencies);

	return (
		<ProxyMenu
			proxyContextValue={{
				proxy: getPreferredProxy(MockWorkspaceProxies, undefined),
				proxies: MockWorkspaceProxies,
				proxyLatencies: latencies,
				latenciesLoaded: true,
				isLoading: false,
				isFetched: true,
				setProxy: vi.fn(),
				clearProxy: vi.fn(),
				refetchProxyLatencies: () => {
					const refetchDate = new Date(Date.now() + 1_000);
					finishRefresh = () => {
						setLatencies(
							Object.fromEntries(
								Object.entries(latencies).map(([id, latency]) => [
									id,
									{ ...latency, at: refetchDate },
								]),
							),
						);
					};
					return refetchDate;
				},
			}}
		/>
	);
};

describe("ProxyMenu", () => {
	it("announces when latencies are refreshing and refreshed", async () => {
		const user = userEvent.setup();
		renderWithAuth(<ProxyMenuWithRefresh />);
		await waitForLoaderToBeRemoved();

		const status = screen.getByRole("status");
		expect(status).toBeEmptyDOMElement();

		await user.click(screen.getByRole("button", { name: /Latency for/ }));
		await user.click(
			screen.getByRole("menuitem", { name: "Refresh latencies" }),
		);
		expect(status).toHaveTextContent("Refreshing latencies");

		act(() => finishRefresh());
		expect(status).toHaveTextContent("Latencies refreshed");
	});
});
