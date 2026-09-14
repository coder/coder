import type { Meta, StoryObj } from "@storybook/react-vite";
import type {
	ComponentProps,
	FC,
	HTMLAttributes,
	PropsWithChildren,
} from "react";
import { PaginationContainer } from "./PaginationContainer";
import {
	mockInitialRenderResult,
	mockPaginationResultBase,
} from "./PaginationContainer.mocks";

// Filtering out optional <div> props to give better auto-complete experience
type EssentialComponent = FC<
	Omit<
		ComponentProps<typeof PaginationContainer>,
		keyof HTMLAttributes<HTMLDivElement>
	> &
		PropsWithChildren
>;

const meta: Meta<EssentialComponent> = {
	title: "components/PaginationContainer",
	component: PaginationContainer,
	args: {
		paginationUnitLabel: "puppies",
		children: <div>Put any content here</div>,
	},
};

export default meta;
type Story = StoryObj<EssentialComponent>;

export const FirstPageBeforeFetch: Story = {
	args: {
		query: mockInitialRenderResult,
	},
};

export const FirstPageWithData: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 1,
			currentOffsetStart: 1,
			totalRecords: 100,
			totalPages: 4,
			hasPreviousPage: false,
			hasNextPage: true,
			isPlaceholderData: false,
		},
	},
};

export const FirstPageWithLittleData: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 1,
			currentOffsetStart: 1,
			totalRecords: 7,
			totalPages: 1,
			hasPreviousPage: false,
			hasNextPage: false,
			isPlaceholderData: false,
		},
	},
};

export const FirstPageWithNoData: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 1,
			currentOffsetStart: 1,
			totalRecords: 0,
			totalPages: 0,
			hasPreviousPage: false,
			hasNextPage: false,
			isPlaceholderData: false,
		},
	},
};

export const FirstPageWithTonsOfData: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 2,
			currentOffsetStart: 1000,
			totalRecords: 123_456,
			totalPages: 4939,
			hasPreviousPage: false,
			hasNextPage: true,
			isPlaceholderData: false,
		},
	},
};

export const TransitionFromFirstToSecondPage: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 2,
			currentOffsetStart: 26,
			totalRecords: 100,
			totalPages: 4,
			hasPreviousPage: false,
			hasNextPage: false,
			isPlaceholderData: true,
		},
		children: <div>Previous data from page 1</div>,
	},
};

export const SecondPageWithData: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 2,
			currentOffsetStart: 26,
			totalRecords: 100,
			totalPages: 4,
			hasPreviousPage: true,
			hasNextPage: true,
			isPlaceholderData: false,
		},
		children: <div>New data for page 2</div>,
	},
};

export const CappedCountFirstPage: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 1,
			currentOffsetStart: 1,
			totalRecords: 2000,
			totalPages: 80,
			hasPreviousPage: false,
			hasNextPage: true,
			isPlaceholderData: false,
			countIsCapped: true,
		},
	},
};

export const CappedCountMiddlePage: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 3,
			currentOffsetStart: 51,
			totalRecords: 2000,
			totalPages: 80,
			hasPreviousPage: true,
			hasNextPage: true,
			isPlaceholderData: false,
			countIsCapped: true,
		},
	},
};

export const CappedCountBeyondKnownPages: Story = {
	args: {
		query: {
			...mockPaginationResultBase,
			isSuccess: true,
			currentPage: 85,
			currentOffsetStart: 2101,
			totalRecords: 2000,
			totalPages: 85,
			hasPreviousPage: true,
			hasNextPage: true,
			isPlaceholderData: false,
			countIsCapped: true,
		},
	},
};
