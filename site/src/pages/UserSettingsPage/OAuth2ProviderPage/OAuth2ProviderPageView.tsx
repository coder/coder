import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import type { FC } from "react";

type OAuth2ProviderPageViewProps = {
	isLoading: boolean;
	error: unknown;
	apps?: TypesGen.OAuth2ProviderApp[];
	revoke: (app: TypesGen.OAuth2ProviderApp) => void;
};

const OAuth2ProviderPageView: FC<OAuth2ProviderPageViewProps> = ({
	isLoading,
	error,
	apps,
	revoke,
}) => {
	return (
		<>
			{error && <ErrorAlert error={error} />}

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Name</TableHead>
						<TableHead className="w-[1%]" />
					</TableRow>
				</TableHeader>
				<TableBody>
					<ChooseOne>
						<Cond condition={isLoading}>
							<TableLoader />
						</Cond>
						<Cond condition={apps === null || apps?.length === 0}>
							<TableEmpty message="No OAuth2 applications have been authorized" />
						</Cond>
						<Cond>
							{apps?.map((app) => (
								<OAuth2AppRow key={app.id} app={app} revoke={revoke} />
							))}
						</Cond>
					</ChooseOne>
				</TableBody>
			</Table>
		</>
	);
};

type OAuth2AppRowProps = {
	app: TypesGen.OAuth2ProviderApp;
	revoke: (app: TypesGen.OAuth2ProviderApp) => void;
};

const OAuth2AppRow: FC<OAuth2AppRowProps> = ({ app, revoke }) => {
	return (
		<TableRow key={app.id} data-testid={`app-${app.id}`}>
			<TableCell>
				<Stack direction="row" spacing={1} alignItems="center">
					<Avatar variant="icon" src={app.icon} fallback={app.name} />
					<span className="font-semibold">{app.name}</span>
				</Stack>
			</TableCell>

			<TableCell>
				<Button size="sm" variant="destructive" onClick={() => revoke(app)}>
					Revoke&hellip;
				</Button>
			</TableCell>
		</TableRow>
	);
};

export default OAuth2ProviderPageView;
