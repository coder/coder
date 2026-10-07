import { HttpResponse, http } from "msw";
import { QueryClient } from "react-query";
import { MockUserOwner } from "#/testHelpers/entities";
import { server } from "#/testHelpers/server";
import {
	getSelfUserFilterOptions,
	getUserFilterOptions,
} from "./userFilterOptions";

const me = { ...MockUserOwner, username: "alice", avatar_url: "/alice.png" };
const bob = { ...MockUserOwner, id: "bob", username: "bob" };

describe("getUserFilterOptions", () => {
	it("puts the current user first and commits the me sentinel", async () => {
		server.use(
			http.get("/api/v2/users", () => HttpResponse.json({ users: [] })),
		);
		const options = await getUserFilterOptions("", me, new QueryClient());
		expect(options[0]).toMatchObject({ label: "alice (you)", value: "me" });
	});

	it("drops the current user from the fetched list to avoid a duplicate", async () => {
		server.use(
			http.get("/api/v2/users", () => HttpResponse.json({ users: [me, bob] })),
		);
		const options = await getUserFilterOptions("", me, new QueryClient());
		expect(options.map((option) => option.value)).toEqual(["me", "bob"]);
	});

	it("matches the current user by its label when the users API does not return it", async () => {
		server.use(
			http.get("/api/v2/users", () => HttpResponse.json({ users: [] })),
		);
		const queryClient = new QueryClient();
		expect(await getUserFilterOptions("ali", me, queryClient)).toHaveLength(1);
		expect(await getUserFilterOptions("zzz", me, queryClient)).toHaveLength(0);
	});

	it("lists the current user when the users API matches it by name or email", async () => {
		server.use(
			http.get("/api/v2/users", () => HttpResponse.json({ users: [me, bob] })),
		);
		const options = await getUserFilterOptions("smith", me, new QueryClient());
		expect(options.map((option) => option.value)).toEqual(["me", "bob"]);
	});
});

describe("getSelfUserFilterOptions", () => {
	it("returns only the current user without fetching", async () => {
		const options = await getSelfUserFilterOptions("", me);
		expect(options).toMatchObject([{ label: "alice (you)", value: "me" }]);
	});

	it("matches the self option by the me sentinel and username", async () => {
		expect(await getSelfUserFilterOptions("me", me)).toHaveLength(1);
		expect(await getSelfUserFilterOptions("ali", me)).toHaveLength(1);
		expect(await getSelfUserFilterOptions("bob", me)).toHaveLength(0);
	});
});
