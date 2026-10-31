import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { CommandGroup } from "#/components/Command/Command";
import { timeZones } from "#/utils/timeZones";

const timezoneGroups = Object.entries(
	Object.groupBy(timeZones, (zone) =>
		zone.includes("/") ? zone.split("/")[0] : "Other",
	),
).sort(([a], [b]) => a.localeCompare(b, "en"));

type TimezoneComboboxProps = {
	id: string;
	value: string;
	onValueChange: (value: string) => void;
	disabled?: boolean;
};

/** Selects a timezone by searching or browsing region groups. */
export const TimezoneCombobox = ({
	id,
	value,
	onValueChange,
	disabled,
}: TimezoneComboboxProps) => (
	<Combobox
		value={value}
		onValueChange={(zone) => {
			if (zone) {
				onValueChange(zone);
			}
		}}
	>
		<ComboboxTrigger asChild>
			<ComboboxButton
				id={id}
				type="button"
				disabled={disabled}
				placeholder="Select timezone"
				selectedOption={
					value ? { value, label: value.replaceAll("_", " ") } : undefined
				}
			/>
		</ComboboxTrigger>
		<ComboboxContent
			label="Search timezones"
			align="start"
			className="w-(--radix-popover-trigger-width)"
		>
			<ComboboxInput placeholder="Search timezones..." />
			<ComboboxList className="max-h-72">
				<ComboboxEmpty>No timezones found.</ComboboxEmpty>
				{timezoneGroups.map(([region, zones]) => (
					<CommandGroup key={region} heading={region} className="p-0">
						{zones?.map((zone) => (
							<ComboboxItem
								key={zone}
								value={zone}
								keywords={[zone.replaceAll("_", " ")]}
							>
								<span className="flex-1 truncate">
									{zone.replaceAll("_", " ")}
								</span>
							</ComboboxItem>
						))}
					</CommandGroup>
				))}
			</ComboboxList>
		</ComboboxContent>
	</Combobox>
);
