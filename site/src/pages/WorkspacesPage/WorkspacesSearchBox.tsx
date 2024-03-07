/**
 * @file Defines a controlled searchbox component for processing form state.
 *
 * Not defined as a top-level component just yet, because it's not clear how
 * reusable this is outside of workspace dropdowns.
 */
import {
  type FC,
  type KeyboardEvent,
  type InputHTMLAttributes,
  type Ref,
  useId,
} from "react";
import { Search, SearchInput } from "components/Search/Search";

interface SearchBoxProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  value: string;
  onKeyDown?: (event: KeyboardEvent) => void;
  onValueChange: (newValue: string) => void;
  $$ref?: Ref<HTMLInputElement>;
}

export const SearchBox: FC<SearchBoxProps> = ({
  onValueChange,
  onKeyDown,
  label = "Search",
  placeholder = "Search...",
  $$ref,
  ...attrs
}) => {
  const hookId = useId();
  const inputId = `${hookId}-${SearchBox.name}-input`;

  return (
    <Search>
      <SearchInput
        label={label}
        $$ref={$$ref}
        id={inputId}
        autoFocus
        tabIndex={0}
        placeholder={placeholder}
        {...attrs}
        onKeyDown={onKeyDown}
        onChange={(e) => onValueChange(e.target.value)}
      />
    </Search>
  );
};
