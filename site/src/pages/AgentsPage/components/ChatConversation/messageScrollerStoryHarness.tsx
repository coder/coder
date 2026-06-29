import type { Decorator } from "@storybook/react-vite";
import { ChatMessageScroller } from "../ChatMessageScroller";

// Renders a story inside the real chat scroll container so components that
// depend on the MessageScroller provider (transcript rows, the live-stream
// row, the jump-to-latest control) behave exactly as they do in the app.
// The fixed-height flex wrapper gives the viewport room to scroll.
export const withMessageScroller: Decorator = (Story) => (
	<div className="flex h-[600px] flex-col">
		<ChatMessageScroller
			hasMoreMessages={false}
			isFetchingMoreMessages={false}
			onFetchMoreMessages={() => {}}
		>
			<Story />
		</ChatMessageScroller>
	</div>
);
