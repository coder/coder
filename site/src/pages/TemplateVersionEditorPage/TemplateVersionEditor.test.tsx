import { MockTemplate, MockTemplateVersion } from "testHelpers/entities";
import { render } from "@testing-library/react";
import type { ProvisionerJobLog, TemplateVersion } from "api/typesGenerated";
import type { ReactNode } from "react";
import type { FileTree } from "utils/filetree";

const {
	capturedUseTemplateAgentOptionsRef,
	templateAgentResetBuildStateMock,
	templateAgentStopMock,
	useQueryMock,
} = vi.hoisted(() => ({
	capturedUseTemplateAgentOptionsRef: {
		current: undefined as
			| {
					onBuildRequested?: () => Promise<void>;
					waitForBuildComplete?: () => Promise<{
						status: "succeeded" | "failed" | "canceled" | "timeout";
						error?: string;
						logs: string;
					}>;
			  }
			| undefined,
	},
	templateAgentResetBuildStateMock: vi.fn(),
	templateAgentStopMock: vi.fn(),
	useQueryMock: vi.fn(),
}));

vi.mock("react-query", () => ({
	useQuery: useQueryMock,
}));

vi.mock("react-router", () => ({
	Link: ({ children, ...props }: Record<string, unknown>) => (
		<a {...props}>{children as ReactNode}</a>
	),
	useNavigate: () => vi.fn(),
	unstable_usePrompt: () => undefined,
}));

vi.mock("hooks/useEmbeddedMetadata", () => ({
	useEmbeddedMetadata: () => ({ metadata: { experiments: [] } }),
}));

vi.mock("modules/navigation", () => ({
	linkToTemplate: (_organization: string, template: string) =>
		`/templates/${template}`,
	useLinks: () => (path: string) => path,
}));

vi.mock("sonner", () => ({
	toast: { error: vi.fn() },
}));

vi.mock("react-resizable-panels", () => ({
	Panel: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
	PanelGroup: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
	PanelResizeHandle: () => <div />,
}));

vi.mock("components/Alert/Alert", () => ({
	Alert: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
}));
vi.mock("components/Button/Button", () => ({
	Button: ({ children, ...props }: Record<string, unknown>) => (
		<button type="button" {...props}>
			{children as ReactNode}
		</button>
	),
}));
vi.mock("components/FullPageLayout/Sidebar", () => ({
	Sidebar: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
}));
vi.mock("components/FullPageLayout/Topbar", () => ({
	Topbar: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
	TopbarAvatar: () => <div />,
	TopbarButton: ({ children, ...props }: Record<string, unknown>) => (
		<button type="button" {...props}>
			{children as ReactNode}
		</button>
	),
	TopbarData: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
	TopbarDivider: () => <div />,
	TopbarIconButton: ({ children, ...props }: Record<string, unknown>) => (
		<button type="button" {...props}>
			{children as ReactNode}
		</button>
	),
}));
vi.mock("components/Loader/Loader", () => ({
	Loader: () => <div />,
}));
vi.mock("components/Tooltip/Tooltip", () => ({
	Tooltip: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
	TooltipContent: ({ children }: { children?: ReactNode }) => (
		<div>{children}</div>
	),
	TooltipTrigger: ({ children }: { children?: ReactNode }) => (
		<div>{children}</div>
	),
}));

vi.mock("modules/provisioners/ProvisionerAlert", () => ({
	AlertVariant: { Info: "info" },
	ProvisionerAlert: () => <div />,
}));
vi.mock("modules/provisioners/ProvisionerStatusAlert", () => ({
	ProvisionerStatusAlert: () => <div />,
}));
vi.mock("modules/resources/WildcardHostnameWarning", () => ({
	WildcardHostnameWarning: () => <div />,
}));
vi.mock("modules/templates/TemplateFiles/isBinaryData", () => ({
	isBinaryData: () => false,
}));
vi.mock("modules/templates/TemplateFiles/TemplateFileTree", () => ({
	TemplateFileTree: () => <div />,
}));
vi.mock(
	"modules/templates/TemplateResourcesTable/TemplateResourcesTable",
	() => ({
		TemplateResourcesTable: () => <div />,
	}),
);
vi.mock("modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs", () => ({
	WorkspaceBuildLogs: () => <div />,
}));

vi.mock("./ai/AIChatPanel", () => ({
	AIChatPanel: () => <div />,
}));
vi.mock("./ai/ModelConfigBar", () => ({
	getDefaultModelConfig: (model: unknown) => ({ model }),
	isCuratedModel: () => false,
}));
vi.mock("./ai/useTemplateAgent", () => ({
	useTemplateAgent: (options: unknown) => {
		capturedUseTemplateAgentOptionsRef.current = options as {
			onBuildRequested?: () => Promise<void>;
			waitForBuildComplete?: () => Promise<{
				status: "succeeded" | "failed" | "canceled" | "timeout";
				error?: string;
				logs: string;
			}>;
		};
		return {
			approve: vi.fn(),
			isStreaming: false,
			messages: [],
			pendingApproval: null,
			reject: vi.fn(),
			reset: vi.fn(),
			resetBuildState: templateAgentResetBuildStateMock,
			send: vi.fn(),
			status: "idle",
			stop: templateAgentStopMock,
		};
	},
}));
vi.mock("./FileDialog", () => ({
	CreateFileDialog: () => <div />,
	DeleteFileDialog: () => <div />,
	RenameFileDialog: () => <div />,
}));
vi.mock("./MissingTemplateVariablesDialog", () => ({
	MissingTemplateVariablesDialog: () => <div />,
}));
vi.mock("./MonacoEditor", () => ({
	MonacoEditor: () => <textarea readOnly value="" />,
}));
vi.mock("./ProvisionerTagsPopover", () => ({
	ProvisionerTagsPopover: () => <div />,
}));
vi.mock("./PublishTemplateVersionDialog", () => ({
	PublishTemplateVersionDialog: () => <div />,
}));
vi.mock("./TemplateVersionStatusBadge", () => ({
	TemplateVersionStatusBadge: () => <div />,
}));

import { TemplateVersionEditor } from "./TemplateVersionEditor";

const defaultFileTree: FileTree = {
	"main.tf": "terraform {}",
};

const defaultBuildLogs: ProvisionerJobLog[] = [
	{
		created_at: "2024-01-01T00:00:00.000Z",
		id: 1,
		log_level: "info",
		log_source: "provisioner",
		output: "Build completed",
		stage: "plan",
	},
];

const renderTemplateVersionEditor = ({
	templateVersion,
	buildLogs = defaultBuildLogs,
	onPreview = vi.fn().mockResolvedValue(templateVersion),
}: {
	templateVersion: TemplateVersion;
	buildLogs?: ProvisionerJobLog[];
	onPreview?: (files: FileTree) => Promise<TemplateVersion>;
}) => {
	useQueryMock.mockImplementation((query: { enabled?: boolean }) => {
		if (query.enabled === false) {
			return { data: [] };
		}
		return { data: [] };
	});

	const view = render(
		<TemplateVersionEditor
			activePath="main.tf"
			buildLogs={buildLogs}
			canPublish={false}
			defaultFileTree={defaultFileTree}
			onActivePathChange={vi.fn()}
			onCancelPublish={vi.fn()}
			onConfirmPublish={vi.fn()}
			onCreateWorkspace={vi.fn()}
			onPreview={onPreview}
			onPublish={vi.fn()}
			onPublishVersion={vi.fn().mockResolvedValue(undefined)}
			onSubmitMissingVariableValues={vi.fn()}
			onCancelSubmitMissingVariableValues={vi.fn()}
			onUpdateProvisionerTags={vi.fn()}
			isAskingPublishParameters={false}
			isBuilding={false}
			isPromptingMissingVariables={false}
			isPublishing={false}
			provisionerTags={{}}
			template={MockTemplate}
			templateVersion={templateVersion}
		/>,
	);

	const getOptions = () => {
		const options = capturedUseTemplateAgentOptionsRef.current;
		expect(options?.onBuildRequested).toBeDefined();
		expect(options?.waitForBuildComplete).toBeDefined();
		return options!;
	};

	return {
		...view,
		getOptions,
		onPreview,
	};
};

describe("TemplateVersionEditor waitForBuildComplete", () => {
	beforeEach(() => {
		capturedUseTemplateAgentOptionsRef.current = undefined;
		templateAgentResetBuildStateMock.mockReset();
		templateAgentStopMock.mockReset();
		useQueryMock.mockReset();
	});

	it("resolves immediately from the newly requested terminal build snapshot", async () => {
		const requestedBuildVersion: TemplateVersion = {
			...MockTemplateVersion,
			id: "test-template-version-2",
			name: "test-version-2",
			job: {
				...MockTemplateVersion.job,
				status: "succeeded",
			},
		};
		const { getOptions } = renderTemplateVersionEditor({
			templateVersion: MockTemplateVersion,
			onPreview: vi.fn().mockResolvedValue(requestedBuildVersion),
		});
		const options = getOptions();

		await options.onBuildRequested?.();
		await expect(options.waitForBuildComplete?.()).resolves.toEqual({
			status: "succeeded",
			error: requestedBuildVersion.job.error,
			logs: "",
		});
	});

	it("does not resolve from a stale previous terminal snapshot", async () => {
		const previousVersion: TemplateVersion = {
			...MockTemplateVersion,
			job: {
				...MockTemplateVersion.job,
				status: "succeeded",
			},
		};
		const requestedBuildVersion: TemplateVersion = {
			...MockTemplateVersion,
			id: "test-template-version-2",
			name: "test-version-2",
			job: {
				...MockTemplateVersion.job,
				status: "pending",
			},
		};
		const completedBuildVersion: TemplateVersion = {
			...requestedBuildVersion,
			job: {
				...requestedBuildVersion.job,
				status: "succeeded",
			},
		};
		const nextBuildLogs: ProvisionerJobLog[] = [
			{
				...defaultBuildLogs[0],
				id: 2,
				output: "New build completed",
			},
		];
		const onPreview = vi.fn().mockResolvedValue(requestedBuildVersion);
		const { getOptions, rerender } = renderTemplateVersionEditor({
			templateVersion: previousVersion,
			onPreview,
		});
		const options = getOptions();

		await options.onBuildRequested?.();
		const buildResultPromise = options.waitForBuildComplete?.();
		expect(buildResultPromise).toBeDefined();

		let didResolve = false;
		buildResultPromise?.then(() => {
			didResolve = true;
		});
		await Promise.resolve();
		expect(didResolve).toBe(false);

		rerender(
			<TemplateVersionEditor
				activePath="main.tf"
				buildLogs={nextBuildLogs}
				canPublish={false}
				defaultFileTree={defaultFileTree}
				onActivePathChange={vi.fn()}
				onCancelPublish={vi.fn()}
				onConfirmPublish={vi.fn()}
				onCreateWorkspace={vi.fn()}
				onPreview={onPreview}
				onPublish={vi.fn()}
				onPublishVersion={vi.fn().mockResolvedValue(undefined)}
				onSubmitMissingVariableValues={vi.fn()}
				onCancelSubmitMissingVariableValues={vi.fn()}
				onUpdateProvisionerTags={vi.fn()}
				isAskingPublishParameters={false}
				isBuilding={false}
				isPromptingMissingVariables={false}
				isPublishing={false}
				provisionerTags={{}}
				template={MockTemplate}
				templateVersion={completedBuildVersion}
			/>,
		);

		await expect(buildResultPromise).resolves.toEqual({
			status: "succeeded",
			error: completedBuildVersion.job.error,
			logs: "[info] plan: New build completed",
		});
	});

	it("maps an unknown terminal snapshot to a failed build result", async () => {
		const requestedBuildVersion: TemplateVersion = {
			...MockTemplateVersion,
			id: "test-template-version-2",
			name: "test-version-2",
			job: {
				...MockTemplateVersion.job,
				error: undefined,
				status: "unknown",
			},
		};
		const { getOptions } = renderTemplateVersionEditor({
			templateVersion: MockTemplateVersion,
			buildLogs: [
				{
					...defaultBuildLogs[0],
					output: "Build status was unknown",
				},
			],
			onPreview: vi.fn().mockResolvedValue(requestedBuildVersion),
		});
		const options = getOptions();

		await options.onBuildRequested?.();
		await expect(options.waitForBuildComplete?.()).resolves.toEqual({
			status: "failed",
			error: "Build ended with an unknown status.",
			logs: "",
		});
	});
});
