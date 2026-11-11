import { Link } from "react-router";
import { Button } from "#/components/Button/Button";
import { NotFoundIllustration } from "#/components/EmptyState/components/NotFoundIllustration";
import { EmptyState } from "#/components/EmptyState/EmptyState";

const NotFoundPage: React.FC = () => {
	return (
		<div className="w-full h-full flex justify-center items-center">
			<EmptyState
				image={<NotFoundIllustration className="-mb-5 h-auto max-w-full" />}
				message="Page not found"
				description="This page doesn't exist or may have been moved."
				cta={
					<Button asChild>
						<Link to="/">Go to dashboard</Link>
					</Button>
				}
			/>
		</div>
	);
};

export default NotFoundPage;
