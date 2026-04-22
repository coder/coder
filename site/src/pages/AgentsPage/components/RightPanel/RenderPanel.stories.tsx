import type { Spec } from "@json-render/react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { RenderPanel } from "./RenderPanel";

const meta: Meta<typeof RenderPanel> = {
	title: "pages/AgentsPage/RightPanel/RenderPanel",
	component: RenderPanel,
	decorators: [
		(Story) => (
			<div className="h-[600px] w-[480px] border border-solid border-border-default bg-surface-primary">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof RenderPanel>;

const dashboardSpec: Spec = {
	root: "layout",
	elements: {
		layout: {
			type: "Card",
			props: {
				title: "Workspace Overview",
				description:
					"Current status of your development workspace and recent activity.",
			},
			children: [
				"statusRow",
				"divider1",
				"metricsRow",
				"divider2",
				"alert",
				"divider3",
				"recentTable",
			],
		},
		statusRow: {
			type: "StatusIndicator",
			props: { label: "Workspace Running", status: "online" },
		},
		divider1: { type: "Divider", props: {} },
		metricsRow: {
			type: "Card",
			props: { title: "Key Metrics" },
			children: ["metric1", "metric2", "metric3"],
		},
		metric1: {
			type: "Metric",
			props: { label: "Active Sessions", value: "12", change: "+3 today" },
		},
		metric2: {
			type: "Metric",
			props: { label: "CPU Usage", value: "67%", change: "stable" },
		},
		metric3: {
			type: "Metric",
			props: {
				label: "Memory",
				value: "4.2 GB",
				change: "of 8 GB allocated",
			},
		},
		divider2: { type: "Divider", props: {} },
		alert: {
			type: "Alert",
			props: {
				message:
					"Template update available. Version 2.4.1 includes performance improvements.",
				severity: "info",
			},
		},
		divider3: { type: "Divider", props: {} },
		recentTable: {
			type: "DataTable",
			props: {
				headers: ["Event", "Time", "Status"],
				rows: [
					["Build completed", "2 min ago", "Success"],
					["Git push", "15 min ago", "Success"],
					["Test run", "1 hour ago", "Failed"],
					["Deployment", "3 hours ago", "Success"],
				],
			},
		},
	},
};

export const Default: Story = {
	args: {
		title: "Workspace Overview",
		spec: dashboardSpec,
	},
};

export const Loading: Story = {
	args: {
		title: "Loading View",
		spec: null,
		isLoading: true,
	},
};

export const Empty: Story = {
	args: {
		title: "Empty View",
		spec: null,
		isLoading: false,
	},
};

const progressSpec: Spec = {
	root: "layout",
	elements: {
		layout: {
			type: "Card",
			props: {
				title: "Build Progress",
				description: "Tracking your workspace build steps.",
			},
			children: [
				"heading",
				"progress1",
				"progress2",
				"progress3",
				"divider",
				"badges",
				"code",
			],
		},
		heading: {
			type: "Heading",
			props: { text: "Step Status", level: 3 },
		},
		progress1: {
			type: "ProgressBar",
			props: { label: "Downloading image", percent: 100 },
		},
		progress2: {
			type: "ProgressBar",
			props: { label: "Installing dependencies", percent: 72 },
		},
		progress3: {
			type: "ProgressBar",
			props: { label: "Running tests", percent: 15 },
		},
		divider: { type: "Divider", props: {} },
		badges: {
			type: "Card",
			props: { title: "Tags" },
			children: ["badge1", "badge2", "badge3", "badge4"],
		},
		badge1: {
			type: "Badge",
			props: { label: "v2.4.0", variant: "default" },
		},
		badge2: {
			type: "Badge",
			props: { label: "Passing", variant: "success" },
		},
		badge3: {
			type: "Badge",
			props: { label: "Slow", variant: "warning" },
		},
		badge4: {
			type: "Badge",
			props: { label: "Deprecated", variant: "error" },
		},
		code: {
			type: "CodeBlock",
			props: {
				language: "bash",
				code: '#!/bin/bash\nset -euo pipefail\n\necho "Starting build..."\nnpm install\nnpm run build\necho "Build complete!"',
			},
		},
	},
};

export const BuildProgress: Story = {
	args: {
		title: "Build Progress",
		spec: progressSpec,
	},
};

const listSpec: Spec = {
	root: "layout",
	elements: {
		layout: {
			type: "Card",
			props: {
				title: "Recommendations",
				description: "Suggestions from AI analysis of your codebase.",
			},
			children: ["text1", "list1", "divider", "text2", "list2"],
		},
		text1: {
			type: "Text",
			props: {
				text: "High priority items to address:",
				emphasis: "strong",
			},
		},
		list1: {
			type: "List",
			props: {
				ordered: true,
				items: [
					"Update dependency express from 4.17.1 to 4.19.2 (security patch)",
					"Add error boundary to WorkspacePage component",
					"Fix N+1 query in GetWorkspacesByOwner",
					"Add rate limiting to /api/v2/users endpoint",
				],
			},
		},
		divider: { type: "Divider", props: {} },
		text2: {
			type: "Text",
			props: {
				text: "Nice to have improvements:",
				emphasis: "subtle",
			},
		},
		list2: {
			type: "List",
			props: {
				ordered: false,
				items: [
					"Consider switching from moment.js to dayjs for smaller bundle",
					"Extract shared form validation into custom hook",
					"Add Storybook stories for TemplateCard component",
				],
			},
		},
	},
};

export const Recommendations: Story = {
	args: {
		title: "AI Recommendations",
		spec: listSpec,
	},
};
