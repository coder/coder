import type {
  ComponentProps,
  FC,
  HTMLAttributes,
  PropsWithChildren,
} from "react";
import { PaginationContainer } from "./PaginationContainer";
import type { Meta, StoryObj } from "@storybook/react";

import {
  mockPaginationResultBase,
  mockInitialRenderResult,
} from "./PaginationContainer.test";

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
    autoScroll: false,
    paginationUnitLabel: "puppies",
    children: <div>Put any content here</div>,
  },
};

export default meta;
type Story = StoryObj<EssentialComponent>;

export const FirstPageBeforeFetch: Story = {
  args: {
    paginationResult: mockInitialRenderResult,
  },
};

export const FirstPageWithData: Story = {
  args: {
    paginationResult: {
      ...mockPaginationResultBase,
      isSuccess: true,
      currentPage: 1,
      currentOffsetStart: 1,
      totalRecords: 100,
      totalPages: 4,
      hasPreviousPage: false,
      hasNextPage: true,
      isPreviousData: false,
    },
  },
};

export const FirstPageWithLittleData: Story = {
  args: {
    paginationResult: {
      ...mockPaginationResultBase,
      isSuccess: true,
      currentPage: 1,
      currentOffsetStart: 1,
      totalRecords: 7,
      totalPages: 1,
      hasPreviousPage: false,
      hasNextPage: false,
      isPreviousData: false,
    },
  },
};

export const FirstPageWithNoData: Story = {
  args: {
    paginationResult: {
      ...mockPaginationResultBase,
      isSuccess: true,
      currentPage: 1,
      currentOffsetStart: 1,
      totalRecords: 0,
      totalPages: 0,
      hasPreviousPage: false,
      hasNextPage: false,
      isPreviousData: false,
    },
  },
};

export const TransitionFromFirstToSecondPage: Story = {
  args: {
    paginationResult: {
      ...mockPaginationResultBase,
      isSuccess: true,
      currentPage: 2,
      currentOffsetStart: 26,
      totalRecords: 100,
      totalPages: 4,
      hasPreviousPage: false,
      hasNextPage: false,
      isPreviousData: true,
    },
  },
};

export const SecondPageWithData: Story = {
  args: {
    paginationResult: {
      ...mockPaginationResultBase,
      isSuccess: true,
      currentPage: 2,
      currentOffsetStart: 26,
      totalRecords: 100,
      totalPages: 4,
      hasPreviousPage: true,
      hasNextPage: true,
      isPreviousData: false,
    },
  },
};
