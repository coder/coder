import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "components/Table/Table";
import { ChevronRight } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

// biome-ignore lint/suspicious/noEmptyInterface: TODO
interface RequestLogsPageViewProps {

}

export const RequestLogsPageView: FC<RequestLogsPageViewProps> = () => {
	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead></TableHead>
					<TableHead>Timestamp</TableHead>
					<TableHead>User</TableHead>
					<TableHead>Prompt</TableHead>
					<TableHead>Tokens</TableHead>
					<TableHead>Tool Calls</TableHead>
					<TableHead>Status</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody>
				{new Array(5).fill(0).map((_, x) => (
					<>
						<TableRow className={cn("cursor-pointer hover:bg-surface-secondary")} key={x}>
							<TableCell>
								<div css={{ display: "flex", alignItems: "center", justifyContent: "center" }}>
									<ChevronRight size={16} />
								</div>
							</TableCell>
							<TableCell>2025-01-01 00:00:00</TableCell>
							<TableCell>John Doe</TableCell>
							<TableCell>This is a prompt</TableCell>
							<TableCell>100</TableCell>
							<TableCell>1</TableCell>
							<TableCell>Status</TableCell>
						</TableRow>
						<TableRow>
							<TableCell colSpan={999}>Foo</TableCell>
						</TableRow>
					</>
				))}
			</TableBody>
		</Table>
	);
};
