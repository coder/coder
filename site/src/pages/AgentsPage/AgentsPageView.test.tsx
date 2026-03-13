import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as TypesGen from "api/typesGenerated";
import { type ReactNode, useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { type AgentsOutletContext, AgentsPageView } from "./AgentsPageView";

let configureMountCount = 0;
let analyticsMountCount = 0;

vi.mock("react-router", async () => {
	const actual =
		await vi.importActual<typeof import("react-router")>("react-router");
	return {
		...actual,
		NavLink: ({ children }: { children: ReactNode }) => <div>{children}</div>,
		Outlet: () => null,
		useOutlet: () => null,
	};
});

vi.mock("./AgentsSidebar", () => ({
	AgentsSidebar: ({
		onOpenAnalytics,
		onOpenSettings,
	}: {
		onOpenAnalytics?: () => void;
		onOpenSettings?: () => void;
	}) => (
		<div>
			<button type="button" onClick={onOpenAnalytics}>
				Open analytics
			</button>
			<button type="button" onClick={onOpenSettings}>
				Open settings
			</button>
		</div>
	),
}));

vi.mock("./ConfigureAgentsDialog", () => ({
	ConfigureAgentsDialog: ({
		open,
		onOpenChange,
		initialSection,
	}: {
		open: boolean;
		onOpenChange: (open: boolean) => void;
		initialSection?: string;
	}) => {
		const [mountId] = useState(() => {
			configureMountCount += 1;
			return configureMountCount;
		});

		if (!open) {
			return null;
		}

		return (
			<div data-testid="configure-dialog" data-mount-id={mountId}>
				section:{initialSection ?? "behavior"}
				<button type="button" onClick={() => onOpenChange(false)}>
					Close configure
				</button>
			</div>
		);
	},
}));

vi.mock("./UserAnalyticsDialog", () => ({
	UserAnalyticsDialog: ({
		open,
		onOpenChange,
	}: {
		open: boolean;
		onOpenChange: (open: boolean) => void;
	}) => {
		const [mountId] = useState(() => {
			analyticsMountCount += 1;
			return analyticsMountCount;
		});

		if (!open) {
			return null;
		}

		return (
			<div data-testid="user-analytics-dialog" data-mount-id={mountId}>
				analytics
				<button type="button" onClick={() => onOpenChange(false)}>
					Close analytics
				</button>
			</div>
		);
	},
}));

vi.mock("./AgentCreateForm", () => ({
	AgentCreateForm: () => <div data-testid="agent-create-form" />,
}));

vi.mock("./ChimeButton", () => ({
	ChimeButton: () => <div />,
}));

vi.mock("./WebPushButton", () => ({
	WebPushButton: () => <div />,
}));

const outletContext: AgentsOutletContext = {
	chatErrorReasons: {},
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
	requestArchiveAgent: vi.fn(),
	requestUnarchiveAgent: vi.fn(),
	requestArchiveAndDeleteWorkspace: vi.fn(),
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: vi.fn(),
};

const defaultProps = {
	agentId: undefined,
	chatList: [] as TypesGen.Chat[],
	catalogModelOptions: [],
	modelConfigs: [] as TypesGen.ChatModelConfig[],
	logoUrl: "",
	handleNewAgent: vi.fn(),
	isCreating: false,
	isArchiving: false,
	archivingChatId: undefined,
	isChatsLoading: false,
	chatsLoadError: null,
	onRetryChatsLoad: vi.fn(),
	onCollapseSidebar: vi.fn(),
	isSidebarCollapsed: false,
	onExpandSidebar: vi.fn(),
	outletContext,
	onCreateChat: vi.fn(),
	createError: undefined,
	modelCatalog: undefined,
	isModelCatalogLoading: false,
	isModelConfigsLoading: false,
	modelCatalogError: undefined,
	hasNextPage: false,
	onLoadMore: vi.fn(),
	isFetchingNextPage: false,
	archivedFilter: "active" as const,
	onArchivedFilterChange: vi.fn(),
};

describe("AgentsPageView analytics entrypoint", () => {
	beforeEach(() => {
		configureMountCount = 0;
		analyticsMountCount = 0;
		vi.clearAllMocks();
	});

	it("opens a separate analytics modal for admins", async () => {
		const user = userEvent.setup();
		render(<AgentsPageView {...defaultProps} isAgentsAdmin />);

		await user.click(screen.getByRole("button", { name: /open analytics/i }));

		expect(screen.getByTestId("user-analytics-dialog")).toBeInTheDocument();
		expect(screen.queryByTestId("configure-dialog")).not.toBeInTheDocument();
	});

	it("opens a separate analytics modal for non-admins", async () => {
		const user = userEvent.setup();
		render(<AgentsPageView {...defaultProps} isAgentsAdmin={false} />);

		await user.click(screen.getByRole("button", { name: /open analytics/i }));

		expect(screen.getByTestId("user-analytics-dialog")).toBeInTheDocument();
		expect(screen.queryByTestId("configure-dialog")).not.toBeInTheDocument();
	});

	it("still opens settings for admins", async () => {
		const user = userEvent.setup();
		render(<AgentsPageView {...defaultProps} isAgentsAdmin />);

		await user.click(screen.getByRole("button", { name: /open settings/i }));

		expect(screen.getByTestId("configure-dialog")).toHaveTextContent(
			"section:behavior",
		);
		expect(
			screen.queryByTestId("user-analytics-dialog"),
		).not.toBeInTheDocument();
	});

	it("still opens settings for non-admins", async () => {
		const user = userEvent.setup();
		render(<AgentsPageView {...defaultProps} isAgentsAdmin={false} />);

		await user.click(screen.getByRole("button", { name: /open settings/i }));

		expect(screen.getByTestId("configure-dialog")).toHaveTextContent(
			"section:behavior",
		);
	});
});

describe("AgentsPageView keyed dialog remounts", () => {
	beforeEach(() => {
		configureMountCount = 0;
		analyticsMountCount = 0;
		vi.clearAllMocks();
	});

	it("remounts the configure dialog when reopened", async () => {
		const user = userEvent.setup();
		render(<AgentsPageView {...defaultProps} isAgentsAdmin />);

		await user.click(screen.getByRole("button", { name: /open settings/i }));

		const initialMountId = screen
			.getByTestId("configure-dialog")
			.getAttribute("data-mount-id");

		await user.click(screen.getByRole("button", { name: /close configure/i }));
		expect(screen.queryByTestId("configure-dialog")).not.toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: /open settings/i }));

		const reopenedMountId = screen
			.getByTestId("configure-dialog")
			.getAttribute("data-mount-id");

		expect(reopenedMountId).not.toBe(initialMountId);
	});

	it("remounts the analytics dialog when reopened", async () => {
		const user = userEvent.setup();
		render(<AgentsPageView {...defaultProps} isAgentsAdmin />);

		await user.click(screen.getByRole("button", { name: /open analytics/i }));

		const initialMountId = screen
			.getByTestId("user-analytics-dialog")
			.getAttribute("data-mount-id");

		await user.click(screen.getByRole("button", { name: /close analytics/i }));
		expect(
			screen.queryByTestId("user-analytics-dialog"),
		).not.toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: /open analytics/i }));

		const reopenedMountId = screen
			.getByTestId("user-analytics-dialog")
			.getAttribute("data-mount-id");

		expect(reopenedMountId).not.toBe(initialMountId);
	});
});
