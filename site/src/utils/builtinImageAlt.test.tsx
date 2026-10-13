import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { getBuiltinImageAlt, useBuiltinImageAlt } from "./builtinImageAlt";

const createWrapper = (): React.FC<React.PropsWithChildren> => {
	const queryClient = createTestQueryClient();
	return ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

describe("getBuiltinImageAlt", () => {
	it.each([
		["/icon/k8s.svg", "Kubernetes logo"],
		["/icon/code.svg", "Visual Studio Code logo"],
		["/icon/jupyter.svg", "Jupyter logo"],
		["/icon/docker-white.svg", "Docker logo"],
		["/icon/argo-workflows.svg", "Argo Workflows logo"],
		["/icon/new-tool.svg?v=2", "New Tool logo"],
		["/icon/folder.svg", "Folder icon"],
		["/icon/terminal.svg", "Terminal icon"],
	])("describes the built-in icon %s as %s", (src, alt) => {
		expect(getBuiltinImageAlt(src)).toBe(alt);
	});

	it("describes built-in emojis by name", () => {
		const emojiNames = { "1f34c": "Banana", "2764-fe0f": "Red Heart" };
		expect(getBuiltinImageAlt("/emojis/1f34c.png", emojiNames)).toBe(
			"Banana emoji",
		);
		expect(getBuiltinImageAlt("/emojis/2764-fe0f.png", emojiNames)).toBe(
			"Red Heart emoji",
		);
		expect(getBuiltinImageAlt("/emojis/1f9ff.png", emojiNames)).toBe("Emoji");
	});

	it.each([undefined, "", "https://avatars.githubusercontent.com/u/1"])(
		"is decorative for %s",
		(src) => {
			expect(getBuiltinImageAlt(src)).toBe("");
		},
	);
});

describe("useBuiltinImageAlt", () => {
	it("resolves emoji names before reporting ready", async () => {
		const srcs = ["/emojis/1f34c.png", "/icon/k8s.svg"];
		const { result } = renderHook(() => useBuiltinImageAlt(srcs), {
			wrapper: createWrapper(),
		});

		expect(result.current.isLoading).toBe(true);
		await waitFor(() => expect(result.current.isLoading).toBe(false));
		expect(result.current.getAlt("/emojis/1f34c.png")).toBe("Banana emoji");
		expect(result.current.getAlt("/icon/k8s.svg")).toBe("Kubernetes logo");
		expect(result.current.getAlt("https://example.com/a.png")).toBe("");
	});

	it("skips loading emoji names when no built-in emoji is present", () => {
		const { result } = renderHook(
			() => useBuiltinImageAlt(["/icon/jupyter.svg"]),
			{ wrapper: createWrapper() },
		);

		expect(result.current.isLoading).toBe(false);
		expect(result.current.getAlt("/icon/jupyter.svg")).toBe("Jupyter logo");
	});
});
