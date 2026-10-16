import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import { Link } from "#/components/Link/Link";
import {
	Alert,
	type AlertColor,
	AlertDescription,
	type AlertDetail,
	AlertTitle,
} from "./Alert";

const meta: Meta<typeof Alert> = {
	title: "components/Alert",
	component: Alert,
};

export default meta;
type Story = StoryObj<typeof Alert>;

const severities: readonly AlertColor[] = [
	"info",
	"success",
	"warning",
	"error",
];

const ExampleAction = (
	<Button onClick={() => null} size="sm">
		Button
	</Button>
);

const ExampleActions = (
	<>
		<Button onClick={() => null} size="sm">
			Retry
		</Button>
		<Button onClick={() => null} size="sm" variant="outline">
			Cancel
		</Button>
	</>
);

const ExampleLink = (
	<Link size="sm" href="https://coder.com/docs">
		View docs
	</Link>
);

const TwoPart = (
	<>
		<AlertTitle>Template version wasn't published</AlertTitle>
		<AlertDescription>
			The Terraform plan failed because the AWS provider version isn't pinned.
			Pin the provider in main.tf and publish again.
		</AlertDescription>
	</>
);

const exampleDetails: readonly AlertDetail[] = [
	{ label: "Request ID", value: "4f1c2a9e-7b3d-4e8a-9c21-5d6f0a1b2c3d" },
	{ label: "Status", value: "500 Internal Server Error" },
	{ label: "Request", value: "POST /api/v2/workspaces" },
	{ label: "Message", value: "provisioner job failed: exit status 1" },
	{ label: "Time", value: "Fri, 16 Oct 2026 17:00:00 GMT" },
];

const expandDetails: Story["play"] = async ({ canvasElement }) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Technical details" }),
	);
};

export const Success: Story = {
	args: {
		children: "You're doing great!",
		severity: "success",
	},
};

export const Warning: Story = {
	args: {
		children: "This is a warning",
		severity: "warning",
	},
};

export const WarningWithDismiss: Story = {
	args: {
		children: "This is a warning",
		dismissible: true,
		severity: "warning",
	},
};

export const WarningWithAction: Story = {
	args: {
		children: "This is a warning",
		actions: [ExampleAction],
		severity: "warning",
	},
};

export const WarningWithActionAndDismiss: Story = {
	args: {
		children: "This is a warning",
		actions: [ExampleAction],
		dismissible: true,
		severity: "warning",
	},
};

export const Info: Story = {
	args: {
		children: "This is an informational message",
		severity: "info",
	},
};

export const ErrorSeverity: Story = {
	args: {
		children: "This is an error message",
		severity: "error",
	},
};

export const WarningProminent: Story = {
	args: {
		children:
			"This is a high risk warning. Use this design only for high risk warnings.",
		severity: "warning",
		prominent: true,
		dismissible: true,
	},
};

export const ErrorProminent: Story = {
	args: {
		children:
			"This is a crucial error. Use this design only for crucial errors.",
		severity: "error",
		prominent: true,
		dismissible: true,
	},
};

export const TitleAndDescription: Story = {
	render: () => (
		<div className="flex flex-col gap-3">
			{severities.map((severity) => (
				<Alert key={severity} severity={severity}>
					{TwoPart}
				</Alert>
			))}
		</div>
	),
};

export const WithTwoActionsAndDismiss: Story = {
	args: {
		children: TwoPart,
		actions: ExampleActions,
		dismissible: true,
		severity: "info",
	},
};

export const WithLinkAction: Story = {
	args: {
		children: TwoPart,
		actions: ExampleLink,
		dismissible: true,
		severity: "info",
	},
};

export const ProminentWithAction: Story = {
	render: () => (
		<div className="flex flex-col gap-3">
			{(["warning", "error"] as const).map((severity) => (
				<Alert
					key={severity}
					severity={severity}
					prominent
					dismissible
					actions={ExampleAction}
				>
					{TwoPart}
				</Alert>
			))}
		</div>
	),
};

export const Compact: Story = {
	render: () => (
		<div className="flex flex-col gap-3">
			{severities.map((severity) => (
				<Alert key={severity} severity={severity} compact>
					This workspace stops automatically in 30 minutes.
				</Alert>
			))}
		</div>
	),
};

export const CompactWithLinkAndDismiss: Story = {
	render: () => (
		<div className="flex flex-col gap-3">
			{severities.map((severity) => (
				<Alert
					key={severity}
					severity={severity}
					compact
					dismissible
					actions={ExampleLink}
				>
					This workspace stops automatically in 30 minutes.
				</Alert>
			))}
		</div>
	),
};

export const CompactProminent: Story = {
	render: () => (
		<div className="flex flex-col gap-3">
			{(["warning", "error"] as const).map((severity) => (
				<Alert
					key={severity}
					severity={severity}
					prominent
					compact
					dismissible
					actions={ExampleLink}
				>
					This workspace stops automatically in 30 minutes.
				</Alert>
			))}
		</div>
	),
};

export const ErrorWithDetails: Story = {
	args: {
		children: TwoPart,
		severity: "error",
		details: exampleDetails,
		actions: ExampleActions,
		dismissible: true,
	},
};

export const ErrorWithDetailsExpanded: Story = {
	args: ErrorWithDetails.args,
	play: expandDetails,
};

export const ErrorProminentWithDetailsExpanded: Story = {
	args: {
		...ErrorWithDetails.args,
		prominent: true,
		actions: ExampleAction,
	},
	play: expandDetails,
};

const AllVariantsSection: React.FC<
	React.PropsWithChildren<{ label: string }>
> = ({ label, children }) => (
	<section className="flex flex-col gap-3">
		<h3 className="m-0 text-xs font-medium text-content-secondary">{label}</h3>
		{children}
	</section>
);

export const AllVariants: Story = {
	render: () => (
		<div className="flex flex-col gap-8">
			<AllVariantsSection label="Default with actions">
				{severities.map((severity) => (
					<Alert key={severity} severity={severity} actions={ExampleActions}>
						{TwoPart}
					</Alert>
				))}
			</AllVariantsSection>
			<AllVariantsSection label="Prominent">
				{(["warning", "error"] as const).map((severity) => (
					<Alert
						key={severity}
						severity={severity}
						prominent
						actions={ExampleAction}
					>
						{TwoPart}
					</Alert>
				))}
			</AllVariantsSection>
			<AllVariantsSection label="Compact">
				{severities.map((severity) => (
					<Alert key={severity} severity={severity} compact>
						This workspace stops automatically in 30 minutes.
					</Alert>
				))}
			</AllVariantsSection>
			<AllVariantsSection label="Compact prominent">
				{(["warning", "error"] as const).map((severity) => (
					<Alert key={severity} severity={severity} prominent compact>
						This workspace stops automatically in 30 minutes.
					</Alert>
				))}
			</AllVariantsSection>
			<AllVariantsSection label="Technical details, collapsed and expanded">
				<Alert
					severity="error"
					details={exampleDetails}
					actions={ExampleActions}
				>
					{TwoPart}
				</Alert>
				<Alert
					severity="error"
					details={exampleDetails}
					actions={ExampleActions}
				>
					{TwoPart}
				</Alert>
				<Alert
					severity="error"
					prominent
					details={exampleDetails}
					actions={ExampleAction}
				>
					{TwoPart}
				</Alert>
				<Alert
					severity="error"
					prominent
					details={exampleDetails}
					actions={ExampleAction}
				>
					{TwoPart}
				</Alert>
			</AllVariantsSection>
			<AllVariantsSection label="Link action and dismiss">
				{severities.map((severity) => (
					<Alert
						key={severity}
						severity={severity}
						actions={ExampleLink}
						dismissible
					>
						{TwoPart}
					</Alert>
				))}
				{severities.map((severity) => (
					<Alert
						key={`${severity}-compact`}
						severity={severity}
						actions={ExampleLink}
						compact
						dismissible
					>
						This workspace stops automatically in 30 minutes.
					</Alert>
				))}
			</AllVariantsSection>
		</div>
	),
	// Expands the second alert of each technical details pair.
	play: async ({ canvasElement }) => {
		const triggers = within(canvasElement).getAllByRole("button", {
			name: "Technical details",
		});
		for (const index of [1, 3]) {
			await userEvent.click(triggers[index]);
		}
	},
};
