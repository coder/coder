import { NotFoundIllustration } from "#/components/EmptyState/components/NotFoundIllustration";
import { EmptyState } from "#/components/EmptyState/EmptyState";

const NotFoundPage: React.FC = () => {
	return (
		<div className="w-full h-full flex justify-center items-center">
			<EmptyState
				image={<NotFoundIllustration className="-mb-5 max-w-full" />}
				message="Page not found"
				description="This page could not be found."
			/>
		</div>
	);
};

export default NotFoundPage;
