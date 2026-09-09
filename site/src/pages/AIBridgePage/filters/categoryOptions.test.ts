import { API } from "#/api/api";
import {
	MockAIProviderAnthropic,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import {
	getClientFilterOptions,
	getInitiatorFilterOptions,
	getModelFilterOptions,
	getProviderFilterOptions,
	type OptionsQueryClient,
} from "./categoryOptions";

const fakeQueryClient = <T>(data: T): OptionsQueryClient => ({
	fetchQuery: (async () => data) as OptionsQueryClient["fetchQuery"],
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("getInitiatorFilterOptions", () => {
	const me = { username: "alice", avatar_url: "/alice.png" };

	it("puts the current user first and commits initiator:me", async () => {
		const queryClient = fakeQueryClient({ users: [] });

		const options = await getInitiatorFilterOptions("", me, queryClient);

		expect(options[0]).toMatchObject({ label: "alice (you)", value: "me" });
	});

	it("drops the current user from the fetched list to avoid a duplicate", async () => {
		const queryClient = fakeQueryClient({
			users: [
				{ username: "alice", avatar_url: "/alice.png" },
				{ username: "bob", avatar_url: "/bob.png" },
			],
		});

		const options = await getInitiatorFilterOptions("", me, queryClient);

		expect(options.map((option) => option.value)).toEqual(["me", "bob"]);
	});

	it("hides the self option when the query matches neither the username nor the me sentinel", async () => {
		const queryClient = fakeQueryClient({
			users: [{ username: "bob", avatar_url: "/bob.png" }],
		});

		expect(
			(await getInitiatorFilterOptions("ali", me, queryClient)).map(
				(option) => option.value,
			),
		).toEqual(["me", "bob"]);
		expect(
			(await getInitiatorFilterOptions("bob", me, queryClient)).map(
				(option) => option.value,
			),
		).toEqual(["bob"]);
	});
});

describe("getProviderFilterOptions", () => {
	it("labels providers by display name and filters them locally", async () => {
		vi.spyOn(API.experimental, "listAIProviders").mockResolvedValue([
			MockAIProviderOpenAI,
			MockAIProviderAnthropic,
		]);

		expect(await getProviderFilterOptions("")).toMatchObject([
			{
				label: MockAIProviderOpenAI.display_name,
				value: MockAIProviderOpenAI.name,
			},
			{
				label: MockAIProviderAnthropic.display_name,
				value: MockAIProviderAnthropic.name,
			},
		]);
		expect(
			(await getProviderFilterOptions("anthro")).map((option) => option.value),
		).toEqual([MockAIProviderAnthropic.name]);
	});
});

describe("getClientFilterOptions", () => {
	it("passes the query to the clients endpoint", async () => {
		const getClients = vi
			.spyOn(API, "getAIBridgeClients")
			.mockResolvedValue(["Claude Code"]);

		const options = await getClientFilterOptions("Cla");

		expect(getClients).toHaveBeenCalledWith({ q: "Cla", limit: 25 });
		expect(options).toMatchObject([
			{ label: "Claude Code", value: "Claude Code" },
		]);
	});
});

describe("getModelFilterOptions", () => {
	it("passes the query to the models endpoint", async () => {
		const getModels = vi
			.spyOn(API, "getAIBridgeModels")
			.mockResolvedValue(["gpt-5"]);

		const options = await getModelFilterOptions("gpt");

		expect(getModels).toHaveBeenCalledWith({ q: "gpt", limit: 25 });
		expect(options).toMatchObject([{ label: "gpt-5", value: "gpt-5" }]);
	});
});
