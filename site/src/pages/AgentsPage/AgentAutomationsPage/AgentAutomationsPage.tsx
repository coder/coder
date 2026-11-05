import { useState } from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { AgentPageHeader } from "../components/AgentPageHeader";
import { AgentAutomationsPageView } from "./AgentAutomationsPageView";
import { placeholderAutomations } from "./placeholderAutomations";

const AgentAutomationsPage: React.FC = () => {
	const [automations, setAutomations] = useState(placeholderAutomations);

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:block!">
			<AgentPageHeader />
			<div className="p-4 pt-8">
				<div className="mx-auto w-full max-w-3xl">
					<AgentAutomationsPageView
						automations={automations}
						onEnabledChange={(id, enabled) => {
							setAutomations((current) =>
								current.map((automation) =>
									automation.id === id
										? { ...automation, enabled }
										: automation,
								),
							);
						}}
						onEditAutomation={() => undefined}
						onRunAutomation={() => undefined}
						onViewChats={() => undefined}
						onDeleteAutomation={() => undefined}
					/>
				</div>
			</div>
		</ScrollArea>
	);
};

export default AgentAutomationsPage;
