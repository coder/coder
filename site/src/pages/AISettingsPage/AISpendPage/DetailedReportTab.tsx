export const DetailedReportTab: React.FC = () => {
	return (
		<section className="flex flex-col gap-2">
			<h2 className="m-0 text-xl font-semibold">Generate a detailed report</h2>
			<p className="m-0 max-w-3xl text-sm text-content-secondary">
				Spend for every user, with the providers, models, and clients they used.
				Large reports can take a while to generate.
			</p>
		</section>
	);
};
