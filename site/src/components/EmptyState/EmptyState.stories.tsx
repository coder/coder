import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "#/components/Button/Button";
import { AnnotationIllustration } from "#/components/EmptyState/components/AnnotationIllustration";
import { SearchIllustration } from "#/components/EmptyState/components/SearchIllustration";
import { NoAccessIllustration } from "./components/NoAccessIllustration";
import { NoResultsIllustration } from "./components/NoResultsIllustration";
import { NotFoundIllustration } from "./components/NotFoundIllustration";
import { EmptyState } from "./EmptyState";

const meta: Meta<typeof EmptyState> = {
	title: "components/EmptyState",
	component: EmptyState,
	args: {
		message: "Create your first workspace",
	},
};

export default meta;
type Story = StoryObj<typeof EmptyState>;

export const Example: Story = {
	args: {
		description: "It is easy, just click the button below",
		cta: <Button>Create workspace</Button>,
	},
};

export const Compact: Story = {
	args: {
		description: "It is easy, just click the button below",
		cta: <Button>Create workspace</Button>,
		isCompact: true,
	},
};

export const Annotation: Story = {
	args: {
		image: <AnnotationIllustration className="-mb-5" />,
		message: "Waiting on templates",
		description:
			"Workspaces are personal, customizable environments built from templates. Once your admin adds one, it'll show up here and you'll be ready to start.",
		cta: <Button>Create a template</Button>,
	},
};

export const Search: Story = {
	args: {
		image: <SearchIllustration className="-mb-5 max-w-full" />,
		message: "No results match your filters",
		description: "Broaden your search to explore more options.",
		cta: <Button>Reset filters</Button>,
	},
};

export const AlternateSearch: Story = {
	args: {
		image: <NoResultsIllustration className="-mb-5 max-w-full" />,
		message: "No results match your search",
		description: "All we found were birds. Widen your search to find more.",
		cta: <Button>Reset filters</Button>,
	},
};

export const NotFound: Story = {
	args: {
		image: <NotFoundIllustration className="-mb-5 max-w-full" />,
		message: "Page not found",
		description: "The page you are looking for could not be found.",
	},
};

export const NoAccess: Story = {
	args: {
		image: <NoAccessIllustration className="-mb-5 max-w-full" />,
		message: "Missing permission",
		description: "Contact your admin to request access.",
	},
};
